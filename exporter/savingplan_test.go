package exporter

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	savingsplansTypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
)

func makeSavingsPlanRate(instanceType, rate string, durationSeconds int64) savingsplansTypes.SavingsPlanOfferingRate {
	return savingsplansTypes.SavingsPlanOfferingRate{
		Rate: aws.String(rate),
		SavingsPlanOffering: &savingsplansTypes.ParentSavingsPlanOffering{
			PaymentOption:   savingsplansTypes.SavingsPlanPaymentOptionNoUpfront,
			DurationSeconds: durationSeconds,
			PlanType:        savingsplansTypes.SavingsPlanTypeCompute,
		},
		Properties: []savingsplansTypes.SavingsPlanOfferingRateProperty{
			{
				Name:  aws.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyInstanceType)),
				Value: aws.String(instanceType),
			},
			{
				Name:  aws.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyRegion)),
				Value: aws.String("us-east-1"),
			},
			{
				Name:  aws.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyProductDescription)),
				Value: aws.String("Linux/UNIX"),
			},
		},
	}
}

func TestGetSavingPlanPricing_NilNextToken(t *testing.T) {
	// Bug #1 validation: should not panic when NextToken is nil
	client := &mockSavingsPlansClient{
		DescribeSavingsPlansOfferingRatesFn: func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
				SearchResults: []savingsplansTypes.SavingsPlanOfferingRate{
					makeSavingsPlanRate("m5.large", "0.04", 31536000), // 1 year
				},
				NextToken: nil, // This used to cause a nil pointer dereference
			}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.savingPlanTypes = []string{"Compute"}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getSavingPlanPricing("us-east-1", client, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	// 1 instance × 3 metrics = 3
	if len(results) != 3 {
		t.Fatalf("expected 3 scrape results, got %d", len(results))
	}
	if results[0].SavingPlanDuration != 1 {
		t.Errorf("expected duration=1, got %d", results[0].SavingPlanDuration)
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
					NextToken: aws.String("page2"),
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

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.savingPlanTypes = []string{"Compute"}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getSavingPlanPricing("us-east-1", client, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	// 2 instances × 3 metrics = 6
	if len(results) != 6 {
		t.Fatalf("expected 6 scrape results, got %d", len(results))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}

func TestGetSavingPlanPricing_APIError(t *testing.T) {
	client := &mockSavingsPlansClient{
		DescribeSavingsPlansOfferingRatesFn: func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.savingPlanTypes = []string{"Compute"}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getSavingPlanPricing("us-east-1", client, scrapes)
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

func TestConvertSavingsPlanType(t *testing.T) {
	result := convertSavingsPlanType([]string{"Compute", "EC2Instance"})
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0] != savingsplansTypes.SavingsPlanTypeCompute {
		t.Errorf("expected Compute, got %s", result[0])
	}
	if result[1] != savingsplansTypes.SavingsPlanTypeEc2Instance {
		t.Errorf("expected EC2Instance, got %s", result[1])
	}
}

func TestConvertPropertiesToStruct_AllProperties(t *testing.T) {
	props := []savingsplansTypes.SavingsPlanOfferingRateProperty{
		{Name: aws.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyRegion)), Value: aws.String("us-east-1")},
		{Name: aws.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyInstanceType)), Value: aws.String("m5.large")},
		{Name: aws.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyInstanceFamily)), Value: aws.String("m5")},
		{Name: aws.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyProductDescription)), Value: aws.String("Linux/UNIX")},
		{Name: aws.String(string(savingsplansTypes.SavingsPlanRatePropertyKeyTenancy)), Value: aws.String("shared")},
	}

	result := convertPropertiesToStruct(props)
	if result.Region != "us-east-1" {
		t.Errorf("expected region=us-east-1, got %s", result.Region)
	}
	if result.InstanceType != "m5.large" {
		t.Errorf("expected instanceType=m5.large, got %s", result.InstanceType)
	}
	if result.InstanceFamily != "m5" {
		t.Errorf("expected instanceFamily=m5, got %s", result.InstanceFamily)
	}
	if result.ProductDescription != "Linux/UNIX" {
		t.Errorf("expected productDescription=Linux/UNIX, got %s", result.ProductDescription)
	}
	if result.Tenancy != "shared" {
		t.Errorf("expected tenancy=shared, got %s", result.Tenancy)
	}
}

func TestConvertPropertiesToStruct_NilValues(t *testing.T) {
	props := []savingsplansTypes.SavingsPlanOfferingRateProperty{
		{Name: nil, Value: aws.String("value")},
		{Name: aws.String("key"), Value: nil},
	}
	result := convertPropertiesToStruct(props)
	// Should not panic; all fields empty
	if result.Region != "" || result.InstanceType != "" {
		t.Error("expected empty struct for nil name/value properties")
	}
}

func TestConvertPropertiesToStruct_Empty(t *testing.T) {
	result := convertPropertiesToStruct(nil)
	if result.Region != "" {
		t.Error("expected empty struct for nil properties")
	}
}

func TestSecondsToYears_1Year(t *testing.T) {
	years, err := SecondsToYears(31536000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if years != 1 {
		t.Errorf("expected 1, got %d", years)
	}
}

func TestSecondsToYears_3Years(t *testing.T) {
	years, err := SecondsToYears(94608000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if years != 3 {
		t.Errorf("expected 3, got %d", years)
	}
}

func TestSecondsToYears_Invalid(t *testing.T) {
	_, err := SecondsToYears(63072000) // 2 years
	if err == nil {
		t.Error("expected error for 2-year duration, got nil")
	}
}

func TestSecondsToYears_Zero(t *testing.T) {
	_, err := SecondsToYears(0)
	if err == nil {
		t.Error("expected error for 0 seconds, got nil")
	}
}
