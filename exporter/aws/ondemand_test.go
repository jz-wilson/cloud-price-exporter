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

	results := make([]provider.ScrapeResult, 0, len(scrapes))
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

	results := make([]provider.ScrapeResult, 0, len(scrapes))
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results on HTTP error, got %d", len(results))
	}
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

	results := make([]provider.ScrapeResult, 0, len(scrapes))
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty pricing, got %d", len(results))
	}
}

func TestGetOnDemandPricing_NilEC2Client(t *testing.T) {
	bulkJSON := makeBulkPricingJSON("SKU001", "m5.large", "Linux", "0.096")
	setupBulkPricingServer(t, bulkJSON, http.StatusOK)

	instances := testInstanceStore()
	var errorCount uint64

	scrapes := make(chan provider.ScrapeResult, 100)
	GetOnDemandPricing(context.Background(), "us-east-1", nil, nil, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, instances, &errorCount, scrapes)
	close(scrapes)

	results := make([]provider.ScrapeResult, 0, len(scrapes))
	for r := range scrapes {
		results = append(results, r)
	}

	// 1 instance × 1 AZ (region fallback) × 3 metrics = 3 results
	if len(results) != 3 {
		t.Fatalf("expected 3 scrape results with nil ec2Client, got %d", len(results))
	}
	if results[0].AvailabilityZone != "us-east-1" {
		t.Errorf("expected AZ=us-east-1 (region fallback), got %s", results[0].AvailabilityZone)
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
