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

	results := make([]provider.ScrapeResult, 0, len(scrapes))
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

	results := make([]provider.ScrapeResult, 0, len(scrapes))
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

	instances := testInstanceStore()
	var errorCount uint64

	scrapes := make(chan provider.ScrapeResult, 100)
	GetSpotPricing(context.Background(), "us-east-1", client, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)

	results := make([]provider.ScrapeResult, 0, len(scrapes))
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results on error, got %d", len(results))
	}
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

	// Only match m5.* instances
	scrapes := make(chan provider.ScrapeResult, 100)
	GetSpotPricing(context.Background(), "us-east-1", client, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(`^m5\.`)}, instances, &errorCount, scrapes)
	close(scrapes)

	results := make([]provider.ScrapeResult, 0, len(scrapes))
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

	instances := testInstanceStore()
	var errorCount uint64

	scrapes := make(chan provider.ScrapeResult, 100)
	GetSpotPricing(context.Background(), "us-east-1", client, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)

	results := make([]provider.ScrapeResult, 0, len(scrapes))
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty response, got %d", len(results))
	}
	if errorCount != 0 {
		t.Errorf("expected errorCount=0, got %d", errorCount)
	}
}
