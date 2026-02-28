package aws

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	savingsplansTypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"

	"github.com/jz-wilson/cloud-price-exporter/exporter/provider"
)

func makeSavingsPlanRate(instanceType, rate string, durationSeconds int64) savingsplansTypes.SavingsPlanOfferingRate {
	return savingsplansTypes.SavingsPlanOfferingRate{
		Rate: awssdk.String(rate),
		SavingsPlanOffering: &savingsplansTypes.ParentSavingsPlanOffering{
			PaymentOption:   savingsplansTypes.SavingsPlanPaymentOptionNoUpfront,
			DurationSeconds: durationSeconds,
			PlanType:        savingsplansTypes.SavingsPlanTypeCompute,
		},
		Properties: []savingsplansTypes.SavingsPlanOfferingRateProperty{
			{
				Name:  awssdk.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyInstanceType)),
				Value: awssdk.String(instanceType),
			},
			{
				Name:  awssdk.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyRegion)),
				Value: awssdk.String("us-east-1"),
			},
			{
				Name:  awssdk.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyProductDescription)),
				Value: awssdk.String("Linux/UNIX"),
			},
		},
	}
}

func TestGetSavingPlanPricing_NilNextToken(t *testing.T) {
	client := &mockSavingsPlansClient{
		DescribeSavingsPlansOfferingRatesFn: func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
				SearchResults: []savingsplansTypes.SavingsPlanOfferingRate{
					makeSavingsPlanRate("m5.large", "0.04", 31536000), // 1 year
				},
				NextToken: nil, // used to cause a nil pointer dereference
			}, nil
		},
	}

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetSavingPlanPricing(context.Background(), "us-east-1", client, []string{"Compute"}, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 3) // 1 instance × 3 metrics
	ec2Results := scrapesByName(results, "ec2")
	if len(ec2Results) != 1 {
		t.Fatalf("expected 1 ec2 metric, got %d", len(ec2Results))
	}
	if ec2Results[0].SavingPlanDuration != 1 {
		t.Errorf("expected duration=1 year, got %d", ec2Results[0].SavingPlanDuration)
	}
	if ec2Results[0].Value != 0.04 {
		t.Errorf("expected value=0.04, got %v", ec2Results[0].Value)
	}
}

func TestGetSavingPlanPricing_MultiplePages(t *testing.T) {
	callCount := 0
	client := &mockSavingsPlansClient{
		DescribeSavingsPlansOfferingRatesFn: func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			callCount++
			if callCount == 1 {
				return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
					SearchResults: []savingsplansTypes.SavingsPlanOfferingRate{
						makeSavingsPlanRate("m5.large", "0.04", 31536000),
					},
					NextToken: awssdk.String("page2"),
				}, nil
			}
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
				SearchResults: []savingsplansTypes.SavingsPlanOfferingRate{
					makeSavingsPlanRate("m5.xlarge", "0.08", 94608000), // 3 years
				},
				NextToken: nil,
			}, nil
		},
	}

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetSavingPlanPricing(context.Background(), "us-east-1", client, []string{"Compute"}, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 6) // 2 instances × 3 metrics
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestGetSavingPlanPricing_APIError(t *testing.T) {
	client := &mockSavingsPlansClient{
		DescribeSavingsPlansOfferingRatesFn: func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetSavingPlanPricing(context.Background(), "us-east-1", client, []string{"Compute"}, []string{"Linux/UNIX"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 0)
	if errorCount != 1 {
		t.Errorf("expected errorCount=1, got %d", errorCount)
	}
}

func TestConvertSavingsPlanType(t *testing.T) {
	tests := []struct {
		input []string
		want  []savingsplansTypes.SavingsPlanType
	}{
		{
			input: []string{"Compute", "EC2Instance", "SageMaker"},
			want:  []savingsplansTypes.SavingsPlanType{savingsplansTypes.SavingsPlanTypeCompute, savingsplansTypes.SavingsPlanTypeEc2Instance, savingsplansTypes.SavingsPlanTypeSagemaker},
		},
		{input: []string{}, want: []savingsplansTypes.SavingsPlanType{}},
	}
	for _, tt := range tests {
		result := convertSavingsPlanType(tt.input)
		if len(result) != len(tt.want) {
			t.Errorf("input %v: expected %d types, got %d", tt.input, len(tt.want), len(result))
			continue
		}
		for i := range tt.want {
			if result[i] != tt.want[i] {
				t.Errorf("input %v [%d]: expected %s, got %s", tt.input, i, tt.want[i], result[i])
			}
		}
	}
}

func TestConvertPropertiesToStruct(t *testing.T) {
	t.Run("all properties", func(t *testing.T) {
		props := []savingsplansTypes.SavingsPlanOfferingRateProperty{
			{Name: awssdk.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyRegion)), Value: awssdk.String("us-east-1")},
			{Name: awssdk.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyInstanceType)), Value: awssdk.String("m5.large")},
			{Name: awssdk.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyInstanceFamily)), Value: awssdk.String("m5")},
			{Name: awssdk.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyProductDescription)), Value: awssdk.String("Linux/UNIX")},
			{Name: awssdk.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyTenancy)), Value: awssdk.String("shared")},
		}
		result := convertPropertiesToStruct(props)
		if result.Region != "us-east-1" {
			t.Errorf("Region: expected us-east-1, got %s", result.Region)
		}
		if result.InstanceType != "m5.large" {
			t.Errorf("InstanceType: expected m5.large, got %s", result.InstanceType)
		}
		if result.InstanceFamily != "m5" {
			t.Errorf("InstanceFamily: expected m5, got %s", result.InstanceFamily)
		}
		if result.ProductDescription != "Linux/UNIX" {
			t.Errorf("ProductDescription: expected Linux/UNIX, got %s", result.ProductDescription)
		}
		if result.Tenancy != "shared" {
			t.Errorf("Tenancy: expected shared, got %s", result.Tenancy)
		}
	})

	t.Run("nil name or value", func(t *testing.T) {
		props := []savingsplansTypes.SavingsPlanOfferingRateProperty{
			{Name: nil, Value: awssdk.String("value")},
			{Name: awssdk.String("key"), Value: nil},
		}
		result := convertPropertiesToStruct(props)
		if result.Region != "" || result.InstanceType != "" {
			t.Error("expected empty struct for nil name/value properties")
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		result := convertPropertiesToStruct(nil)
		if result.Region != "" {
			t.Errorf("expected empty struct for nil slice, got Region=%q", result.Region)
		}
	})
}

func TestSecondsToYears(t *testing.T) {
	tests := []struct {
		name      string
		seconds   int64
		wantYears int
		wantErr   bool
	}{
		{name: "1 year", seconds: 31536000, wantYears: 1},
		{name: "3 years", seconds: 94608000, wantYears: 3},
		{name: "2 years — unsupported", seconds: 63072000, wantErr: true},
		{name: "zero", seconds: 0, wantErr: true},
		{name: "negative", seconds: -31536000, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			years, err := SecondsToYears(tt.seconds)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %d seconds, got nil", tt.seconds)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if years != tt.wantYears {
				t.Errorf("expected %d years, got %d", tt.wantYears, years)
			}
		})
	}
}
