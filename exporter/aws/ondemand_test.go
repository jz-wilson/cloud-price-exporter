package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/jz-wilson/cloud-price-exporter/exporter/provider"
)

// makeBulkPricingJSON builds a valid bulk pricing JSON string for testing.
func makeBulkPricingJSON(sku, instanceType, os, priceUSD string) string {
	resp := BulkPricingResponse{
		Products: map[string]BulkProduct{
			sku: {
				SKU:           sku,
				ProductFamily: "Compute Instance",
				Attributes: map[string]string{
					"instanceType":       instanceType,
					"operatingSystem":    os,
					"tenancy":            "Shared",
					"capacitystatus":     "Used",
					"preInstalledSw":     "NA",
					"productDescription": "Linux/UNIX",
				},
			},
		},
		Terms: BulkTerms{
			OnDemand: map[string]map[string]BulkOfferTerm{
				sku: {
					fmt.Sprintf("%s.%s", sku, TermOnDemand): {
						OfferTermCode: TermOnDemand,
						PriceDimensions: map[string]BulkPriceDimension{
							fmt.Sprintf("%s.%s.%s", sku, TermOnDemand, TermPerHour): {
								PricePerUnit: map[string]string{"USD": priceUSD},
							},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

// setupBulkPricingServer starts an httptest server serving the given JSON body
// and overrides BulkPricingURLFormat for the duration of the test.
func setupBulkPricingServer(t *testing.T, body string, statusCode int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	orig := BulkPricingURLFormat
	BulkPricingURLFormat = ts.URL + "/%s"
	t.Cleanup(func() { BulkPricingURLFormat = orig })
	return ts
}

func TestGetOnDemandPricing_SinglePage(t *testing.T) {
	ec2Client := &mockEC2Client{
		DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
			return &ec2.DescribeAvailabilityZonesOutput{
				AvailabilityZones: []ec2types.AvailabilityZone{
					{ZoneName: awssdk.String("us-east-1a")},
					{ZoneName: awssdk.String("us-east-1b")},
				},
			}, nil
		},
	}

	bulkJSON := makeBulkPricingJSON("SKU001", "m5.large", "Linux", "0.096")
	setupBulkPricingServer(t, bulkJSON, http.StatusOK)

	instances := testInstanceStore()
	var errorCount uint64

	scrapes := make(chan provider.ScrapeResult, 100)
	GetOnDemandPricing(context.Background(), "us-east-1", ec2Client, nil, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	// 1 instance × 2 AZs × 3 metrics = 6 results
	requireScrapeCount(t, results, 6)

	// Verify all results share common expected fields
	for i, r := range results {
		if r.InstanceLifecycle != "ondemand" {
			t.Errorf("results[%d]: expected lifecycle=ondemand, got %q", i, r.InstanceLifecycle)
		}
		if r.Region != "us-east-1" {
			t.Errorf("results[%d]: expected region=us-east-1, got %q", i, r.Region)
		}
		if r.InstanceType != "m5.large" {
			t.Errorf("results[%d]: expected instance=m5.large, got %q", i, r.InstanceType)
		}
	}

	// Verify ec2 metric values per AZ
	ec2Results := scrapesByName(results, "ec2")
	if len(ec2Results) != 2 {
		t.Fatalf("expected 2 ec2 metrics (1 per AZ), got %d", len(ec2Results))
	}
	for _, r := range ec2Results {
		if r.Value != 0.096 {
			t.Errorf("ec2 AZ=%s: expected value=0.096, got %v", r.AvailabilityZone, r.Value)
		}
	}
	azs := map[string]bool{ec2Results[0].AvailabilityZone: true, ec2Results[1].AvailabilityZone: true}
	if !azs["us-east-1a"] || !azs["us-east-1b"] {
		t.Errorf("expected AZs us-east-1a and us-east-1b, got %v", azs)
	}
}

func TestGetOnDemandPricing_HTTPError(t *testing.T) {
	ec2Client := &mockEC2Client{
		DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
			return &ec2.DescribeAvailabilityZonesOutput{
				AvailabilityZones: []ec2types.AvailabilityZone{
					{ZoneName: awssdk.String("us-east-1a")},
				},
			}, nil
		},
	}

	setupBulkPricingServer(t, "error", http.StatusInternalServerError)

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetOnDemandPricing(context.Background(), "us-east-1", ec2Client, nil, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 0)
	if errorCount == 0 {
		t.Error("expected errorCount to be incremented on HTTP error")
	}
}

func TestGetOnDemandPricing_EmptyResults(t *testing.T) {
	ec2Client := &mockEC2Client{
		DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
			return &ec2.DescribeAvailabilityZonesOutput{
				AvailabilityZones: []ec2types.AvailabilityZone{
					{ZoneName: awssdk.String("us-east-1a")},
				},
			}, nil
		},
	}

	// Serve an empty bulk pricing response
	emptyResp := BulkPricingResponse{
		Products: map[string]BulkProduct{},
		Terms:    BulkTerms{OnDemand: map[string]map[string]BulkOfferTerm{}},
	}
	emptyJSON, _ := json.Marshal(emptyResp)
	setupBulkPricingServer(t, string(emptyJSON), http.StatusOK)

	instances := testInstanceStore()
	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 100)
	GetOnDemandPricing(context.Background(), "us-east-1", ec2Client, nil, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)

	requireScrapeCount(t, drainScrapes(t, scrapes), 0)
}

func TestGetOnDemandPricing_NilEC2Client(t *testing.T) {
	bulkJSON := makeBulkPricingJSON("SKU001", "m5.large", "Linux", "0.096")
	setupBulkPricingServer(t, bulkJSON, http.StatusOK)

	instances := testInstanceStore()
	var errorCount uint64

	scrapes := make(chan provider.ScrapeResult, 100)
	GetOnDemandPricing(context.Background(), "us-east-1", nil, nil, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)
	results := drainScrapes(t, scrapes)

	// 1 instance × 1 AZ (region fallback) × 3 metrics = 3 results
	requireScrapeCount(t, results, 3)
	for _, r := range results {
		if r.AvailabilityZone != "us-east-1" {
			t.Errorf("expected AZ=us-east-1 (region fallback), got %q", r.AvailabilityZone)
		}
	}
}

func TestGetAZs_Success(t *testing.T) {
	client := &mockEC2Client{
		DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
			return &ec2.DescribeAvailabilityZonesOutput{
				AvailabilityZones: []ec2types.AvailabilityZone{
					{ZoneName: awssdk.String("us-east-1a")},
					{ZoneName: awssdk.String("us-east-1b")},
					{ZoneName: awssdk.String("us-east-1c")},
				},
			}, nil
		},
	}

	azs, err := GetAZs(context.Background(), "us-east-1", client)
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

	_, err := GetAZs(context.Background(), "us-east-1", client)
	if err == nil {
		t.Fatal("expected error from GetAZs, got nil")
	}
}
