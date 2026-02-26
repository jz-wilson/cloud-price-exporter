package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
)

// makePricingJSON builds a valid pricing JSON string for testing.
func makePricingJSON(sku, instanceType, os string, priceUSD string) string {
	p := Pricing{
		Product: Product{
			Sku:           sku,
			ProductFamily: "Compute Instance",
			Attributes: map[string]string{
				"instanceType":       instanceType,
				"operatingSystem":    os,
				"productDescription": "Linux/UNIX",
			},
		},
		ServiceCode: "AmazonEC2",
		Terms: Terms{
			OnDemand: map[string]SKU{
				fmt.Sprintf("%s.%s", sku, TermOnDemand): {
					PriceDimensions: map[string]Details{
						fmt.Sprintf("%s.%s.%s", sku, TermOnDemand, TermPerHour): {
							Unit: "Hrs",
							PricePerUnit: map[string]string{
								"USD": priceUSD,
							},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(p)
	return string(b)
}

func TestGetOnDemandPricing_SinglePage(t *testing.T) {
	ec2Client := &mockEC2Client{
		DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
			return &ec2.DescribeAvailabilityZonesOutput{
				AvailabilityZones: []ec2types.AvailabilityZone{
					{ZoneName: aws.String("us-east-1a")},
					{ZoneName: aws.String("us-east-1b")},
				},
			}, nil
		},
	}

	pricingJSON := makePricingJSON("SKU001", "m5.large", "Linux", "0.096")

	pricingClient := &mockPricingClient{
		GetProductsFn: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			return &pricing.GetProductsOutput{
				PriceList: []string{pricingJSON},
			}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.operatingSystems = []string{"Linux"}
	})

	scrapes := make(chan scrapeResult, 100)
	e.getOnDemandPricing(context.Background(), "us-east-1", ec2Client, pricingClient, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	// 1 instance × 2 AZs × 3 metrics = 6 results
	if len(results) != 6 {
		t.Fatalf("expected 6 scrape results, got %d", len(results))
	}

	// Verify first result
	if results[0].Name != "ec2" {
		t.Errorf("expected name=ec2, got %s", results[0].Name)
	}
	if results[0].Value != 0.096 {
		t.Errorf("expected value=0.096, got %v", results[0].Value)
	}
	if results[0].InstanceLifecycle != "ondemand" {
		t.Errorf("expected lifecycle=ondemand, got %s", results[0].InstanceLifecycle)
	}
	if results[0].AvailabilityZone != "us-east-1a" {
		t.Errorf("expected AZ=us-east-1a, got %s", results[0].AvailabilityZone)
	}
}

func TestGetOnDemandPricing_APIError(t *testing.T) {
	ec2Client := &mockEC2Client{
		DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
			return &ec2.DescribeAvailabilityZonesOutput{
				AvailabilityZones: []ec2types.AvailabilityZone{
					{ZoneName: aws.String("us-east-1a")},
				},
			}, nil
		},
	}

	pricingClient := &mockPricingClient{
		GetProductsFn: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
			return nil, fmt.Errorf("service unavailable")
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.operatingSystems = []string{"Linux"}
	})

	scrapes := make(chan scrapeResult, 100)
	// Bug #2 validation: should not panic on nil pricelist
	e.getOnDemandPricing(context.Background(), "us-east-1", ec2Client, pricingClient, scrapes)
	close(scrapes)

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results on pricing API error, got %d", len(results))
	}
}

func TestGetAZs_Success(t *testing.T) {
	client := &mockEC2Client{
		DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
			return &ec2.DescribeAvailabilityZonesOutput{
				AvailabilityZones: []ec2types.AvailabilityZone{
					{ZoneName: aws.String("us-east-1a")},
					{ZoneName: aws.String("us-east-1b")},
					{ZoneName: aws.String("us-east-1c")},
				},
			}, nil
		},
	}

	e := &Exporter{}
	azs, err := e.getAZs(context.Background(), "us-east-1", client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(azs) != 3 {
		t.Fatalf("expected 3 AZs, got %d", len(azs))
	}
	if azs[0] != "us-east-1a" || azs[1] != "us-east-1b" || azs[2] != "us-east-1c" {
		t.Errorf("unexpected AZs: %v", azs)
	}
}

func TestGetAZs_APIError(t *testing.T) {
	client := &mockEC2Client{
		DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	e := &Exporter{}
	_, err := e.getAZs(context.Background(), "us-east-1", client)
	if err == nil {
		t.Fatal("expected error from getAZs, got nil")
	}
}
