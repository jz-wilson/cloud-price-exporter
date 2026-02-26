package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"sync/atomic"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	log "github.com/sirupsen/logrus"

	"github.com/pixelfederation/cloud-price-exporter/exporter/provider"
)

// GetOnDemandPricing fetches on-demand prices for a region and sends results to scrapes.
func GetOnDemandPricing(ctx context.Context, region string, ec2Client EC2DescribeAZsAPI, pricingClient pricing.GetProductsAPIClient, operatingSystems []string, instanceRegexes []*regexp.Regexp, instances *InstanceStore, errorCount *uint64, scrapes chan<- provider.ScrapeResult) {
	azs, err := GetAZs(ctx, region, ec2Client)
	if err != nil {
		log.WithError(err).Errorf("error while fetching AZs [region=%s]", region)
		atomic.AddUint64(errorCount, 1)
		return
	}

	var outs []Pricing
	for _, os := range operatingSystems {
		pag := pricing.NewGetProductsPaginator(
			pricingClient,
			&pricing.GetProductsInput{
				ServiceCode: awssdk.String("AmazonEC2"),
				MaxResults:  awssdk.Int32(MaxResultsPerPage),
				Filters: []pricingtypes.Filter{
					{
						Field: awssdk.String("regionCode"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: awssdk.String(region),
					},
					{
						Field: awssdk.String("capacitystatus"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: awssdk.String("Used"),
					},
					{
						Field: awssdk.String("tenancy"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: awssdk.String("Shared"),
					},
					{
						Field: awssdk.String("preInstalledSw"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: awssdk.String("NA"),
					},
					{
						Field: awssdk.String("operatingSystem"),
						Type:  pricingtypes.FilterTypeTermMatch,
						Value: awssdk.String(os),
					},
				},
			},
		)
		for pag.HasMorePages() {
			pricelist, err := pag.NextPage(ctx)
			if err != nil {
				log.WithError(err).Errorf("error while fetching ondemand price [region=%s]", region)
				atomic.AddUint64(errorCount, 1)
				break
			}
			for _, price := range pricelist.PriceList {
				var tmp Pricing
				log.Debug(price)
				if err := json.Unmarshal([]byte(price), &tmp); err != nil {
					log.WithError(err).Errorf("failed to unmarshal pricing item [region=%s]", region)
					atomic.AddUint64(errorCount, 1)
					continue
				}
				outs = append(outs, tmp)
			}
		}
	}

	for _, out := range outs {
		if !provider.IsMatchAny(instanceRegexes, out.Product.Attributes["instanceType"]) {
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
			atomic.AddUint64(errorCount, 1)
			continue
		}
		log.Debugf("Creating new metric: ec2{region=%s, instance_type=%s, product_description=%s} = %v.", region, out.Product.Attributes["instanceType"], out.Product.Attributes["operatingSystem"], value)

		vcpu, memory := instances.GetNormalizedCost(value, out.Product.Attributes["instanceType"])
		for _, az := range azs {
			scrapes <- provider.ScrapeResult{
				Name:               "ec2",
				Value:              value,
				Region:             region,
				AvailabilityZone:   az,
				InstanceType:       out.Product.Attributes["instanceType"],
				InstanceLifecycle:  "ondemand",
				OperatingSystem:    out.Product.Attributes["operatingSystem"],
				ProductDescription: out.Product.Attributes["productDescription"],
				Memory:             instances.GetMemory(out.Product.Attributes["instanceType"]),
				VCpu:               instances.GetVCpu(out.Product.Attributes["instanceType"]),
			}
			scrapes <- provider.ScrapeResult{
				Name:              "ec2_memory",
				Value:             memory,
				Region:            region,
				AvailabilityZone:  az,
				InstanceType:      out.Product.Attributes["instanceType"],
				InstanceLifecycle: "ondemand",
			}
			scrapes <- provider.ScrapeResult{
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

// GetAZs returns the availability zone names for a region.
func GetAZs(ctx context.Context, region string, client EC2DescribeAZsAPI) ([]string, error) {
	tmpazs, err := client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2types.Filter{
			{
				Name:   awssdk.String("group-name"),
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
