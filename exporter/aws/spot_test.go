package aws

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/jz-wilson/cloud-price-exporter/exporter/provider"
)

func TestGetSpotPricing_SinglePage(t *testing.T) {
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			return &ec2.DescribeSpotPriceHistoryOutput{
				SpotPriceHistory: []ec2types.SpotPrice{
					{
						InstanceType:       ec2types.InstanceTypeM5Large,
						SpotPrice:          awssdk.String("0.05"),
						AvailabilityZone:   awssdk.String("us-east-1a"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
					{
						InstanceType:       ec2types.InstanceTypeM5Xlarge,
						SpotPrice:          awssdk.String("0.10"),
						AvailabilityZone:   awssdk.String("us-east-1b"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
				},
			}, nil
		},
	}

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetSpotPricing(context.Background(), "us-east-1", client, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	// 2 instances × 3 metrics (ec2, ec2_memory, ec2_vcpu) = 6 results
	requireScrapeCount(t, results, 6)

	// Verify every result has spot lifecycle and correct region
	for i, r := range results {
		if r.InstanceLifecycle != "spot" {
			t.Errorf("results[%d]: expected lifecycle=spot, got %q", i, r.InstanceLifecycle)
		}
		if r.Region != "us-east-1" {
			t.Errorf("results[%d]: expected region=us-east-1, got %q", i, r.Region)
		}
	}

	// Verify the ec2 metric for the first instance
	ec2Results := scrapesByName(results, "ec2")
	if len(ec2Results) != 2 {
		t.Fatalf("expected 2 ec2 metrics, got %d", len(ec2Results))
	}
	if ec2Results[0].InstanceType != "m5.large" || ec2Results[0].Value != 0.05 {
		t.Errorf("first ec2: want instance=m5.large value=0.05, got instance=%s value=%v", ec2Results[0].InstanceType, ec2Results[0].Value)
	}
	if ec2Results[1].InstanceType != "m5.xlarge" || ec2Results[1].Value != 0.10 {
		t.Errorf("second ec2: want instance=m5.xlarge value=0.10, got instance=%s value=%v", ec2Results[1].InstanceType, ec2Results[1].Value)
	}
}

func TestGetSpotPricing_MultiplePages(t *testing.T) {
	callCount := 0
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			callCount++
			if callCount == 1 {
				return &ec2.DescribeSpotPriceHistoryOutput{
					NextToken: awssdk.String("page2"),
					SpotPriceHistory: []ec2types.SpotPrice{
						{
							InstanceType:       ec2types.InstanceTypeM5Large,
							SpotPrice:          awssdk.String("0.05"),
							AvailabilityZone:   awssdk.String("us-east-1a"),
							ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
						},
					},
				}, nil
			}
			return &ec2.DescribeSpotPriceHistoryOutput{
				SpotPriceHistory: []ec2types.SpotPrice{
					{
						InstanceType:       ec2types.InstanceTypeM5Xlarge,
						SpotPrice:          awssdk.String("0.10"),
						AvailabilityZone:   awssdk.String("us-east-1b"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
				},
			}, nil
		},
	}

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetSpotPricing(context.Background(), "us-east-1", client, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 6) // 2 pages × 1 instance × 3 metrics
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestGetSpotPricing_APIError(t *testing.T) {
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			return nil, fmt.Errorf("throttled")
		},
	}

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetSpotPricing(context.Background(), "us-east-1", client, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 0)
	if errorCount != 1 {
		t.Errorf("expected errorCount=1, got %d", errorCount)
	}
}

func TestGetSpotPricing_InstanceRegexFilter(t *testing.T) {
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			return &ec2.DescribeSpotPriceHistoryOutput{
				SpotPriceHistory: []ec2types.SpotPrice{
					{
						InstanceType:       ec2types.InstanceTypeM5Large,
						SpotPrice:          awssdk.String("0.05"),
						AvailabilityZone:   awssdk.String("us-east-1a"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
					{
						InstanceType:       ec2types.InstanceTypeC5Xlarge,
						SpotPrice:          awssdk.String("0.08"),
						AvailabilityZone:   awssdk.String("us-east-1a"),
						ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
					},
				},
			}, nil
		},
	}

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	// Only match m5.* — c5.xlarge must be filtered out
	GetSpotPricing(context.Background(), "us-east-1", client, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(`^m5\.`)}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 3) // m5.large only × 3 metrics
	for _, r := range results {
		if r.InstanceType != "m5.large" {
			t.Errorf("expected only m5.large results, got %q", r.InstanceType)
		}
	}
}

func TestGetSpotPricing_EmptyResponse(t *testing.T) {
	client := &mockEC2Client{
		DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
			return &ec2.DescribeSpotPriceHistoryOutput{}, nil
		},
	}

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetSpotPricing(context.Background(), "us-east-1", client, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 0)
	if errorCount != 0 {
		t.Errorf("expected no errors for empty response, got %d", errorCount)
	}
}

// scrapesByName filters results to those with a matching Name field.
func scrapesByName(results []provider.ScrapeResult, name string) []provider.ScrapeResult {
	var out []provider.ScrapeResult
	for _, r := range results {
		if r.Name == name {
			out = append(out, r)
		}
	}
	return out
}
