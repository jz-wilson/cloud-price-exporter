package aws

import (
	"context"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	log "github.com/sirupsen/logrus"

	"github.com/jz-wilson/cloud-price-exporter/exporter/provider"
)

// GetSpotPricing fetches spot prices for a region and sends results to scrapes.
func GetSpotPricing(ctx context.Context, region string, client ec2.DescribeSpotPriceHistoryAPIClient, productDescriptions []string, instanceRegexes []*regexp.Regexp, instances *InstanceStore, errorCount *uint64, scrapes chan<- provider.ScrapeResult) {
	pag := ec2.NewDescribeSpotPriceHistoryPaginator(
		client,
		&ec2.DescribeSpotPriceHistoryInput{
			StartTime:           awssdk.Time(time.Now()),
			MaxResults:          awssdk.Int32(MaxResultsPerPage),
			ProductDescriptions: productDescriptions,
		})
	for pag.HasMorePages() {
		history, err := pag.NextPage(ctx)
		if err != nil {
			log.WithError(err).Errorf("error while fetching spot price history [region=%s]", region)
			atomic.AddUint64(errorCount, 1)
			break
		}
		for _, price := range history.SpotPriceHistory {
			if !provider.IsMatchAny(instanceRegexes, string(price.InstanceType)) {
				log.Debugf("Skipping instance type: %s", price.InstanceType)
				continue
			}

			value, err := strconv.ParseFloat(*price.SpotPrice, 64)
			if err != nil {
				log.WithError(err).Errorf("error while parsing spot price value from API response [region=%s, az=%s, type=%s]", region, *price.AvailabilityZone, price.InstanceType)
				atomic.AddUint64(errorCount, 1)
				continue
			}
			log.Debugf("Creating new metric: ec2{region=%s, az=%s, instance_type=%s, product_description=%s} = %v.", region, *price.AvailabilityZone, price.InstanceType, price.ProductDescription, value)

			scrapes <- provider.ScrapeResult{
				Name:               "ec2",
				Value:              value,
				Region:             region,
				AvailabilityZone:   *price.AvailabilityZone,
				InstanceType:       string(price.InstanceType),
				InstanceLifecycle:  "spot",
				ProductDescription: string(price.ProductDescription),
				Memory:             instances.GetMemory(string(price.InstanceType)),
				VCpu:               instances.GetVCpu(string(price.InstanceType)),
			}

			vcpu, memory := instances.GetNormalizedCost(value, string(price.InstanceType))
			scrapes <- provider.ScrapeResult{
				Name:              "ec2_memory",
				Value:             memory,
				Region:            region,
				AvailabilityZone:  *price.AvailabilityZone,
				InstanceType:      string(price.InstanceType),
				InstanceLifecycle: "spot",
			}
			scrapes <- provider.ScrapeResult{
				Name:              "ec2_vcpu",
				Value:             vcpu,
				Region:            region,
				AvailabilityZone:  *price.AvailabilityZone,
				InstanceType:      string(price.InstanceType),
				InstanceLifecycle: "spot",
			}
		}
	}
}
