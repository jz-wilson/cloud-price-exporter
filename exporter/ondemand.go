package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	log "github.com/sirupsen/logrus"
)

const (
	TermOnDemand string = "JRTCKXETXF"
	TermPerHour  string = "6YS6EN2CT7"
)

func (e *Exporter) getOnDemandPricing(ctx context.Context, region string, ec2Client EC2DescribeAZsAPI, pricingClient pricing.GetProductsAPIClient, scrapes chan<- scrapeResult) {
	azs, err := e.getAZs(ctx, region, ec2Client)
	if err != nil {
		log.WithError(err).Errorf("error while fetching AZs [region=%s]", region)
		atomic.AddUint64(&e.errorCount, 1)
		return
	}

	var outs []Pricing
	for _, os := range e.operatingSystems {
		pag := pricing.NewGetProductsPaginator(
			pricingClient,
			&pricing.GetProductsInput{
				ServiceCode: aws.String("AmazonEC2"),
				MaxResults:  aws.Int32(AwsMaxResultsPerPage),
				Filters: []pricingtypes.Filter{
					{
						Field: aws.String("regionCode"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: aws.String(region),
					},
					{
						Field: aws.String("capacitystatus"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: aws.String("Used"),
					},
					{
						Field: aws.String("tenancy"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: aws.String("Shared"),
					},
					{
						Field: aws.String("preInstalledSw"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: aws.String("NA"),
					},
					{
						Field: aws.String("operatingSystem"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: aws.String(os),
					},
				},
			},
		)
		for pag.HasMorePages() {
			pricelist, err := pag.NextPage(ctx)
			if err != nil {
				log.WithError(err).Errorf("error while fetching ondemand price [region=%s]", region)
				atomic.AddUint64(&e.errorCount, 1)
				break
			}
			for _, price := range pricelist.PriceList {
				var tmp Pricing
				log.Debug(price)
				if err := json.Unmarshal([]byte(price), &tmp); err != nil {
					log.WithError(err).Errorf("failed to unmarshal pricing item [region=%s]", region)
					atomic.AddUint64(&e.errorCount, 1)
					continue
				}
				outs = append(outs, tmp)
			}
		}
	}

	for _, out := range outs {
		if !isMatchAny(e.instanceRegexes, out.Product.Attributes["instanceType"]) {
			log.Debugf("Skipping instance type: %s", out.Product.Attributes["instanceType"])
			continue
		}

		sku := out.Product.Sku
		skuOnDemand := fmt.Sprintf("%s.%s", sku, TermOnDemand)
		skuOnDemandPerHour := fmt.Sprintf("%s.%s", skuOnDemand, TermPerHour)

		skuEntry, ok := out.Terms.OnDemand[skuOnDemand]
		if !ok {
			continue
		}
		dimEntry, ok := skuEntry.PriceDimensions[skuOnDemandPerHour]
		if !ok {
			continue
		}
		usdPrice, ok := dimEntry.PricePerUnit["USD"]
		if !ok {
			continue
		}
		value, err := strconv.ParseFloat(usdPrice, 64)
		if err != nil {
			log.WithError(err).Errorf("error while parsing ondemand price value from API response [region=%s, type=%s]", region, out.Product.Attributes["instanceType"])
			atomic.AddUint64(&e.errorCount, 1)
			continue
		}
		log.Debugf("Creating new metric: ec2{region=%s, instance_type=%s, product_description=%s} = %v.", region, out.Product.Attributes["instanceType"], out.Product.Attributes["operatingSystem"], value)

		vcpu, memory := e.getNormalizedCost(value, out.Product.Attributes["instanceType"])
		for _, az := range azs {
			scrapes <- scrapeResult{
				Name:               "ec2",
				Value:              value,
				Region:             region,
				AvailabilityZone:   az,
				InstanceType:       out.Product.Attributes["instanceType"],
				InstanceLifecycle:  "ondemand",
				OperatingSystem:    out.Product.Attributes["operatingSystem"],
				ProductDescription: out.Product.Attributes["productDescription"],
				Memory:             e.getInstanceMemory(out.Product.Attributes["instanceType"]),
				VCpu:               e.getInstanceVCpu(out.Product.Attributes["instanceType"]),
			}
			scrapes <- scrapeResult{
				Name:              "ec2_memory",
				Value:             memory,
				Region:            region,
				AvailabilityZone:  az,
				InstanceType:      out.Product.Attributes["instanceType"],
				InstanceLifecycle: "ondemand",
			}
			scrapes <- scrapeResult{
				Name:              "ec2_vcpu",
				Value:             vcpu,
				Region:            region,
				AvailabilityZone:  az,
				InstanceType:      out.Product.Attributes["instanceType"],
				InstanceLifecycle: "ondemand",
			}
		}
	}
}

func (e *Exporter) getAZs(ctx context.Context, region string, client EC2DescribeAZsAPI) ([]string, error) {
	tmpazs, err := client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("group-name"),
				Values: []string{region},
			},
		}})

	if err != nil {
		return nil, fmt.Errorf("couldn't describe AZs in %s: %w", region, err)
	}

	azs := make([]string, len(tmpazs.AvailabilityZones))
	for i, az := range tmpazs.AvailabilityZones {
		azs[i] = *az.ZoneName
	}

	return azs, nil
}
