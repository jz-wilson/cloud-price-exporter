package exporter

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	log "github.com/sirupsen/logrus"
)

func (e *Exporter) getSpotPricing(region string, client ec2.DescribeSpotPriceHistoryAPIClient, scrapes chan<- scrapeResult) {
	pag := ec2.NewDescribeSpotPriceHistoryPaginator(
		client,
		&ec2.DescribeSpotPriceHistoryInput{
			StartTime:           aws.Time(time.Now()),
			MaxResults:          aws.Int32(AwsMaxResultsPerPage),
			ProductDescriptions: e.productDescriptions,
		})
	for pag.HasMorePages() {
		history, err := pag.NextPage(context.TODO())
		if err != nil {
			log.WithError(err).Errorf("error while fetching spot price history [region=%s]", region)
			atomic.AddUint64(&e.errorCount, 1)
			break
		}
		for _, price := range history.SpotPriceHistory {
			if !isMatchAny(e.instanceRegexes, string(price.InstanceType)) {
				log.Debugf("Skipping instance type: %s", price.InstanceType)
				continue
			}

			value, err := strconv.ParseFloat(*price.SpotPrice, 64)
			if err != nil {
				log.WithError(err).Errorf("error while parsing spot price value from API response [region=%s, az=%s, type=%s]", region, *price.AvailabilityZone, price.InstanceType)
				atomic.AddUint64(&e.errorCount, 1)
				continue
			}
			log.Debugf("Creating new metric: ec2{region=%s, az=%s, instance_type=%s, product_description=%s} = %v.", region, *price.AvailabilityZone, price.InstanceType, price.ProductDescription, value)

			scrapes <- scrapeResult{
				Name:               "ec2",
				Value:              value,
				Region:             region,
				AvailabilityZone:   *price.AvailabilityZone,
				InstanceType:       string(price.InstanceType),
				InstanceLifecycle:  "spot",
				ProductDescription: string(price.ProductDescription),
				Memory:             e.getInstanceMemory(string(price.InstanceType)),
				VCpu:               e.getInstanceVCpu(string(price.InstanceType)),
			}

			vcpu, memory := e.getNormalizedCost(value, string(price.InstanceType))
			scrapes <- scrapeResult{
				Name:              "ec2_memory",
				Value:             memory,
				Region:            region,
				AvailabilityZone:  *price.AvailabilityZone,
				InstanceType:      string(price.InstanceType),
				InstanceLifecycle: "spot",
			}
			scrapes <- scrapeResult{
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
