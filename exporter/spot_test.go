package exporter

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestGetSpotPricing_SinglePage(t *testing.T) {
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			return &ec2.DescribeSpotPriceHistoryOutput{
				SpotPriceHistory: []ec2types.SpotPrice{
					{
						InstanceType:       ec2types.InstanceTypeM5Large,
						SpotPrice:          aws.String("0.05"),
						AvailabilityZone:   aws.String("us-east-1a"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
					{
						InstanceType:       ec2types.InstanceTypeM5Xlarge,
						SpotPrice:          aws.String("0.10"),
						AvailabilityZone:   aws.String("us-east-1b"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
				},
			}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getSpotPricing("us-east-1", client, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	// 2 instances × 3 metrics (ec2, ec2_memory, ec2_vcpu) = 6 results
	if len(results) != 6 {
		t.Fatalf("expected 6 scrape results, got %d", len(results))
	}

	// Verify first instance ec2 metric
	if results[0].Name != "ec2" || results[0].Value != 0.05 || results[0].InstanceType != "m5.large" {
		t.Errorf("unexpected first result: %+v", results[0])
	}
	if results[0].InstanceLifecycle != "spot" {
		t.Errorf("expected lifecycle=spot, got %s", results[0].InstanceLifecycle)
	}
}

func TestGetSpotPricing_MultiplePages(t *testing.T) {
	callCount := 0
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			callCount++
			if callCount == 1 {
				return &ec2.DescribeSpotPriceHistoryOutput{
					NextToken: aws.String("page2"),
					SpotPriceHistory: []ec2types.SpotPrice{
						{
							InstanceType:       ec2types.InstanceTypeM5Large,
							SpotPrice:          aws.String("0.05"),
							AvailabilityZone:   aws.String("us-east-1a"),
							ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
						},
					},
				}, nil
			}
			return &ec2.DescribeSpotPriceHistoryOutput{
				SpotPriceHistory: []ec2types.SpotPrice{
					{
						InstanceType:       ec2types.InstanceTypeM5Xlarge,
						SpotPrice:          aws.String("0.10"),
						AvailabilityZone:   aws.String("us-east-1b"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
				},
			}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getSpotPricing("us-east-1", client, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	// 2 pages × 1 instance × 3 metrics = 6 results
	if len(results) != 6 {
		t.Fatalf("expected 6 scrape results, got %d", len(results))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}

func TestGetSpotPricing_APIError(t *testing.T) {
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			return nil, fmt.Errorf("throttled")
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getSpotPricing("us-east-1", client, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results on error, got %d", len(results))
	}
	if e.errorCount != 1 {
		t.Errorf("expected errorCount=1, got %d", e.errorCount)
	}
}

func TestGetSpotPricing_InstanceRegexFilter(t *testing.T) {
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			return &ec2.DescribeSpotPriceHistoryOutput{
				SpotPriceHistory: []ec2types.SpotPrice{
					{
						InstanceType:       ec2types.InstanceTypeM5Large,
						SpotPrice:          aws.String("0.05"),
						AvailabilityZone:   aws.String("us-east-1a"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
					{
						InstanceType:       ec2types.InstanceTypeC5Xlarge,
						SpotPrice:          aws.String("0.08"),
						AvailabilityZone:   aws.String("us-east-1a"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
				},
			}, nil
		},
	}

	// Only match m5.* instances
	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(`^m5\.`)}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getSpotPricing("us-east-1", client, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	// Only m5.large matches, c5.xlarge filtered out → 3 results
	if len(results) != 3 {
		t.Fatalf("expected 3 scrape results (m5.large only), got %d", len(results))
	}
	if results[0].InstanceType != "m5.large" {
		t.Errorf("expected m5.large, got %s", results[0].InstanceType)
	}
}

func TestGetSpotPricing_EmptyResponse(t *testing.T) {
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			return &ec2.DescribeSpotPriceHistoryOutput{}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getSpotPricing("us-east-1", client, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty response, got %d", len(results))
	}
	if e.errorCount != 0 {
		t.Errorf("expected errorCount=0, got %d", e.errorCount)
	}
}
