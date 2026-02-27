package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	savingsplansTypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jz-wilson/cloud-price-exporter/exporter/aws"
	"github.com/jz-wilson/cloud-price-exporter/exporter/azure"
	"github.com/jz-wilson/cloud-price-exporter/exporter/provider"
)

// instancesJSON is a small fake response for ec2instances.info used by tests
// that go through the NewExporter constructor (which calls InstanceStore.Load).
const instancesJSON = `[{"instance_type":"m5.large","vcpu":2,"memory":8.0}]`

// setupInstancesServer starts an httptest server that serves fake instance data
// and overrides aws.EC2InstancesInfoURL for the duration of the test.
func setupInstancesServer(t *testing.T) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(instancesJSON))
	}))
	t.Cleanup(ts.Close)
	orig := aws.EC2InstancesInfoURL
	aws.EC2InstancesInfoURL = ts.URL
	t.Cleanup(func() { aws.EC2InstancesInfoURL = orig })
}

// setupFailingInstancesServer starts an httptest server that always returns 500.
func setupFailingInstancesServer(t *testing.T) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)
	orig := aws.EC2InstancesInfoURL
	aws.EC2InstancesInfoURL = ts.URL
	t.Cleanup(func() { aws.EC2InstancesInfoURL = orig })
}

func newMockFactoryWithInstances() *mockClientFactory {
	return &mockClientFactory{
		ec2Client: &mockEC2Client{
			DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
				return &ec2.DescribeSpotPriceHistoryOutput{
					SpotPriceHistory: []ec2types.SpotPrice{
						{
							InstanceType:       ec2types.InstanceTypeM5Large,
							SpotPrice:          awssdk.String("0.05"),
							AvailabilityZone:   awssdk.String("us-east-1a"),
							ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
						},
					},
				}, nil
			},
			DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
				return &ec2.DescribeAvailabilityZonesOutput{
					AvailabilityZones: []ec2types.AvailabilityZone{
						{ZoneName: awssdk.String("us-east-1a")},
					},
				}, nil
			},
		},
		spClient: &mockSavingsPlansClient{
			DescribeSavingsPlansOfferingRatesFn: func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
				return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{}, nil
			},
		},
	}
}

func TestNewExporter_Success(t *testing.T) {
	setupInstancesServer(t)
	factory := newMockFactoryWithInstances()

	exp, err := NewExporter(
		[]string{"Linux/UNIX"},
		[]string{"Linux"},
		[]string{"us-east-1"},
		[]string{"spot"},
		300,
		[]*regexp.Regexp{regexp.MustCompile(".*")},
		[]string{},
		factory,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp == nil {
		t.Fatal("expected non-nil exporter")
	}
	if exp.instances.Len() != 1 {
		t.Errorf("expected 1 instance, got %d", exp.instances.Len())
	}
}

func TestNewExporter_FailsOnInstancesError(t *testing.T) {
	setupFailingInstancesServer(t)

	factory := &mockClientFactory{}

	_, err := NewExporter(
		[]string{"Linux/UNIX"},
		[]string{"Linux"},
		[]string{"us-east-1"},
		[]string{"spot"},
		300,
		[]*regexp.Regexp{regexp.MustCompile(".*")},
		[]string{},
		factory,
		nil,
	)
	if err == nil {
		t.Fatal("expected error from NewExporter when instance loading fails")
	}
}

func TestCollect_ClientFactoryError(t *testing.T) {
	factory := &mockClientFactory{
		ec2Err: fmt.Errorf("config error"),
	}

	e := newTestExporter(factory, func(e *Exporter) {
		e.lifecycle = []string{"spot"}
		e.nextScrape = time.Now().Add(-1 * time.Second)
	})

	ch := make(chan prometheus.Metric, 100)
	e.Collect(ch)
	close(ch)

	for range ch {
	}

	// Should complete without panic — errors are logged and counted
}

func TestDescribe(t *testing.T) {
	e := newTestExporter(nil)

	ch := make(chan *prometheus.Desc, 100)
	e.Describe(ch)
	close(ch)

	descs := make([]*prometheus.Desc, 0, len(ch))
	for d := range ch {
		descs = append(descs, d)
	}

	// 3 pricing gauges (ec2, ec2_memory, ec2_vcpu) + duration + totalScrapes + scrapeErrors = 6
	if len(descs) != 6 {
		t.Errorf("expected 6 descriptors, got %d", len(descs))
	}
}

func TestCollect_CacheHit(t *testing.T) {
	factory := newMockFactoryWithInstances()

	e := newTestExporter(factory, func(e *Exporter) {
		e.lifecycle = []string{"spot"}
		e.cache = 3600                                  // 1 hour cache
		e.nextScrape = time.Now().Add(-1 * time.Second) // expired, will trigger scrape
	})

	// First collect — triggers scrape
	ch1 := make(chan prometheus.Metric, 100)
	e.Collect(ch1)
	close(ch1)
	count1 := 0
	for range ch1 {
		count1++
	}

	// Second collect — should hit cache (nextScrape is now in the future)
	ch2 := make(chan prometheus.Metric, 100)
	e.Collect(ch2)
	close(ch2)
	count2 := 0
	for range ch2 {
		count2++
	}

	// Both should return the same metrics
	if count1 != count2 {
		t.Errorf("cache hit should return same metrics: first=%d, second=%d", count1, count2)
	}
}

func TestCollect_CacheExpired(t *testing.T) {
	factory := newMockFactoryWithInstances()
	scrapeCount := 0

	// Override the spot client to count scrapes
	factory.ec2Client.(*mockEC2Client).DescribeSpotPriceHistoryFn = func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
		scrapeCount++
		return &ec2.DescribeSpotPriceHistoryOutput{
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

	e := newTestExporter(factory, func(e *Exporter) {
		e.lifecycle = []string{"spot"}
		e.cache = 0 // no caching
		e.nextScrape = time.Now().Add(-1 * time.Second)
	})

	ch1 := make(chan prometheus.Metric, 100)
	e.Collect(ch1)
	close(ch1)
	for range ch1 {
	}

	// Force nextScrape to past
	e.nextScrape = time.Now().Add(-1 * time.Second)

	ch2 := make(chan prometheus.Metric, 100)
	e.Collect(ch2)
	close(ch2)
	for range ch2 {
	}

	if scrapeCount != 2 {
		t.Errorf("expected 2 scrapes (cache expired), got %d", scrapeCount)
	}
}

func TestSetPricingMetrics(t *testing.T) {
	e := newTestExporter(nil)

	scrapes := make(chan provider.ScrapeResult, 10)
	scrapes <- provider.ScrapeResult{
		Name:              "ec2",
		Value:             0.05,
		Region:            "us-east-1",
		AvailabilityZone:  "us-east-1a",
		InstanceType:      "m5.large",
		InstanceLifecycle: "spot",
		Memory:            "8192",
		VCpu:              "2",
	}
	scrapes <- provider.ScrapeResult{
		Name:              "ec2_memory",
		Value:             0.002,
		Region:            "us-east-1",
		AvailabilityZone:  "us-east-1a",
		InstanceType:      "m5.large",
		InstanceLifecycle: "spot",
	}
	scrapes <- provider.ScrapeResult{
		Name:              "ec2_vcpu",
		Value:             0.015,
		Region:            "us-east-1",
		AvailabilityZone:  "us-east-1a",
		InstanceType:      "m5.large",
		InstanceLifecycle: "spot",
	}
	close(scrapes)

	e.setPricingMetrics(scrapes)

	// Verify metrics were set by collecting them
	ch := make(chan prometheus.Metric, 100)
	for _, m := range e.pricingMetrics {
		m.Collect(ch)
	}
	close(ch)

	count := 0
	for range ch {
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 metrics, got %d", count)
	}
}

func TestCollectorContract(t *testing.T) {
	// The prometheus library requires consistency between Describe and Collect.
	// NewRegistry().Register validates this.
	factory := newMockFactoryWithInstances()
	e := newTestExporter(factory, func(e *Exporter) {
		e.lifecycle = []string{"spot"}
		e.cache = 0
		e.nextScrape = time.Now().Add(-1 * time.Second)
	})

	reg := prometheus.NewRegistry()
	err := reg.Register(e)
	if err != nil {
		t.Fatalf("failed to register exporter: %v", err)
	}

	// Gather triggers Collect and validates against Describe
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("expected at least some metrics")
	}
}

func TestContains(t *testing.T) {
	if !provider.Contains([]string{"a", "b", "c"}, "b") {
		t.Error("expected true for 'b' in [a,b,c]")
	}
	if provider.Contains([]string{"a", "b", "c"}, "d") {
		t.Error("expected false for 'd' in [a,b,c]")
	}
	if provider.Contains([]string{}, "a") {
		t.Error("expected false for empty slice")
	}
}

func TestIsMatchAny(t *testing.T) {
	regexes := []*regexp.Regexp{
		regexp.MustCompile(`^m5\.`),
		regexp.MustCompile(`^c5\.`),
	}

	if !provider.IsMatchAny(regexes, "m5.large") {
		t.Error("expected match for m5.large")
	}
	if !provider.IsMatchAny(regexes, "c5.xlarge") {
		t.Error("expected match for c5.xlarge")
	}
	if provider.IsMatchAny(regexes, "r5.large") {
		t.Error("expected no match for r5.large")
	}
	if provider.IsMatchAny(nil, "anything") {
		t.Error("expected no match for nil regex list")
	}
}

func TestDescribe_WithAzure(t *testing.T) {
	e := newTestExporter(nil, func(e *Exporter) {
		e.azureEnabled = true
		e.azureRegions = []string{"eastus"}
		e.azureOperatingSystems = []string{"Linux"}
		e.azureClientFactory = &mockAzureClientFactory{client: &mockAzureRetailPricesClient{}}
	})

	ch := make(chan *prometheus.Desc, 100)
	e.Describe(ch)
	close(ch)

	descs := make([]*prometheus.Desc, 0, len(ch))
	for d := range ch {
		descs = append(descs, d)
	}

	// 3 AWS pricing gauges + 1 azure_vm + duration + totalScrapes + scrapeErrors = 7
	if len(descs) != 7 {
		t.Errorf("expected 7 descriptors with Azure, got %d", len(descs))
	}
}

func TestCollect_AzureOnly(t *testing.T) {
	azureClient := &mockAzureRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]azure.RetailPriceItem, error) {
			return []azure.RetailPriceItem{
				{RetailPrice: 0.096, ArmRegionName: "eastus", ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series", MeterName: "D2s v5"},
			}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.regions = []string{}
		e.lifecycle = []string{}
		e.azureEnabled = true
		e.azureRegions = []string{"eastus"}
		e.azureOperatingSystems = []string{"Linux"}
		e.azureInstanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.azureClientFactory = &mockAzureClientFactory{client: azureClient}
		e.nextScrape = time.Now().Add(-1 * time.Second)
	})

	ch := make(chan prometheus.Metric, 100)
	e.Collect(ch)
	close(ch)

	count := 0
	for range ch {
		count++
	}
	// Should have azure_vm metric(s) + duration + totalScrapes + scrapeErrors
	if count < 4 {
		t.Errorf("expected at least 4 metrics (1 azure_vm + 3 internal), got %d", count)
	}
}

func TestCollect_AWSAndAzure(t *testing.T) {
	awsFactory := newMockFactoryWithInstances()

	azureClient := &mockAzureRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]azure.RetailPriceItem, error) {
			return []azure.RetailPriceItem{
				{RetailPrice: 0.096, ArmRegionName: "eastus", ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series", MeterName: "D2s v5"},
			}, nil
		},
	}

	e := newTestExporter(awsFactory, func(e *Exporter) {
		e.lifecycle = []string{"spot"}
		e.instanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.azureEnabled = true
		e.azureRegions = []string{"eastus"}
		e.azureOperatingSystems = []string{"Linux"}
		e.azureInstanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.azureClientFactory = &mockAzureClientFactory{client: azureClient}
		e.nextScrape = time.Now().Add(-1 * time.Second)
	})

	ch := make(chan prometheus.Metric, 100)
	e.Collect(ch)
	close(ch)

	count := 0
	for range ch {
		count++
	}
	// AWS spot: ec2 + ec2_memory + ec2_vcpu = 3 pricing metrics
	// Azure: 1 azure_vm metric
	// Internal: duration + totalScrapes + scrapeErrors = 3
	// Total: at least 7
	if count < 7 {
		t.Errorf("expected at least 7 metrics (AWS + Azure + internal), got %d", count)
	}
}

func TestCollect_ConcurrentSafety(t *testing.T) {
	factory := newMockFactoryWithInstances()

	e := newTestExporter(factory, func(e *Exporter) {
		e.lifecycle = []string{"spot"}
		e.cache = 0
		e.nextScrape = time.Now().Add(-1 * time.Second)
	})

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := make(chan prometheus.Metric, 100)
			e.Collect(ch)
			close(ch)
			for range ch {
			}
		}()
	}
	wg.Wait()
	// Success: no panic, no data race (verified by -race flag)
}

func TestSetPricingMetrics_UnknownName(t *testing.T) {
	e := newTestExporter(nil)

	scrapes := make(chan provider.ScrapeResult, 1)
	scrapes <- provider.ScrapeResult{
		Name:         "nonexistent",
		Value:        1.0,
		Region:       "us-east-1",
		InstanceType: "m5.large",
	}
	close(scrapes)

	// Should not panic — unknown names are silently dropped with a log warning
	e.setPricingMetrics(scrapes)
}

// findMetricFamily returns the named MetricFamily from gathered output, or nil.
func findMetricFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

// hasLabelValue checks whether at least one metric in the family has the given label=value pair.
func hasLabelValue(family *dto.MetricFamily, label, value string) bool {
	for _, m := range family.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == label && lp.GetValue() == value {
				return true
			}
		}
	}
	return false
}

func TestEndToEnd_SpotPricing(t *testing.T) {
	setupInstancesServer(t)
	factory := newMockFactoryWithInstances()

	exp, err := NewExporter(
		[]string{"Linux/UNIX"},
		[]string{"Linux"},
		[]string{"us-east-1"},
		[]string{"spot"},
		0,
		[]*regexp.Regexp{regexp.MustCompile(".*")},
		[]string{},
		factory,
		nil,
	)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}

	reg := prometheus.NewRegistry()
	err = reg.Register(exp)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	ec2Family := findMetricFamily(families, "aws_pricing_ec2")
	if ec2Family == nil {
		t.Fatal("expected aws_pricing_ec2 metric family")
	}
	if !hasLabelValue(ec2Family, "instance_lifecycle", "spot") {
		t.Error("expected instance_lifecycle=spot label on aws_pricing_ec2")
	}
	if !hasLabelValue(ec2Family, "instance_type", "m5.large") {
		t.Error("expected instance_type=m5.large label on aws_pricing_ec2")
	}
}

// makeBulkPricingJSON builds a valid bulk pricing JSON string for testing.
func makeBulkPricingJSON(sku, instanceType, os, priceUSD string) string {
	resp := aws.BulkPricingResponse{
		Products: map[string]aws.BulkProduct{
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
		Terms: aws.BulkTerms{
			OnDemand: map[string]map[string]aws.BulkOfferTerm{
				sku: {
					fmt.Sprintf("%s.%s", sku, aws.TermOnDemand): {
						OfferTermCode: aws.TermOnDemand,
						PriceDimensions: map[string]aws.BulkPriceDimension{
							fmt.Sprintf("%s.%s.%s", sku, aws.TermOnDemand, aws.TermPerHour): {
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

// setupBulkPricingServer starts an httptest server serving bulk pricing JSON
// and overrides aws.BulkPricingURLFormat for the duration of the test.
func setupBulkPricingServer(t *testing.T, body string) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	orig := aws.BulkPricingURLFormat
	aws.BulkPricingURLFormat = ts.URL + "/%s"
	t.Cleanup(func() { aws.BulkPricingURLFormat = orig })
}

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

func TestEndToEnd_OnDemandPricing(t *testing.T) {
	setupInstancesServer(t)
	bulkJSON := makeBulkPricingJSON("SKU001", "m5.large", "Linux", "0.096")
	setupBulkPricingServer(t, bulkJSON)

	factory := &mockClientFactory{
		ec2Client: &mockEC2Client{
			DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
				return &ec2.DescribeAvailabilityZonesOutput{
					AvailabilityZones: []ec2types.AvailabilityZone{
						{ZoneName: awssdk.String("us-east-1a")},
					},
				}, nil
			},
		},
	}

	exp, err := NewExporter(
		[]string{"Linux/UNIX"},
		[]string{"Linux"},
		[]string{"us-east-1"},
		[]string{"ondemand"},
		0,
		[]*regexp.Regexp{regexp.MustCompile(".*")},
		[]string{},
		factory,
		nil,
	)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}

	reg := prometheus.NewRegistry()
	err = reg.Register(exp)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	ec2Family := findMetricFamily(families, "aws_pricing_ec2")
	if ec2Family == nil {
		t.Fatal("expected aws_pricing_ec2 metric family")
	}
	if !hasLabelValue(ec2Family, "instance_lifecycle", "ondemand") {
		t.Error("expected instance_lifecycle=ondemand label on aws_pricing_ec2")
	}
}

func TestEndToEnd_SavingsPlanPricing(t *testing.T) {
	setupInstancesServer(t)

	factory := &mockClientFactory{
		ec2Client: &mockEC2Client{},
		spClient: &mockSavingsPlansClient{
			DescribeSavingsPlansOfferingRatesFn: func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
				return &savingsplans.DescribeSavingsPlansOfferingRatesOutput{
					SearchResults: []savingsplansTypes.SavingsPlanOfferingRate{
						makeSavingsPlanRate("m5.large", "0.04", 31536000),
					},
				}, nil
			},
		},
	}

	exp, err := NewExporter(
		[]string{"Linux/UNIX"},
		[]string{"Linux"},
		[]string{"us-east-1"},
		[]string{},
		0,
		[]*regexp.Regexp{regexp.MustCompile(".*")},
		[]string{"Compute"},
		factory,
		nil,
	)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}

	reg := prometheus.NewRegistry()
	err = reg.Register(exp)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	ec2Family := findMetricFamily(families, "aws_pricing_ec2")
	if ec2Family == nil {
		t.Fatal("expected aws_pricing_ec2 metric family for savings plan path")
	}
	if !hasLabelValue(ec2Family, "saving_plan_type", "Compute") {
		t.Error("expected saving_plan_type=Compute label on aws_pricing_ec2")
	}
}

func TestEndToEnd_AzurePricing(t *testing.T) {
	azureClient := &mockAzureRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]azure.RetailPriceItem, error) {
			return []azure.RetailPriceItem{
				{RetailPrice: 0.096, ArmRegionName: "eastus", ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series", MeterName: "D2s v5"},
			}, nil
		},
	}

	// No AWS regions — Azure only
	factory := &mockClientFactory{}

	exp, err := NewExporter(
		[]string{},
		[]string{},
		[]string{},
		[]string{},
		0,
		[]*regexp.Regexp{},
		[]string{},
		factory,
		&AzureConfig{
			Regions:          []string{"eastus"},
			OperatingSystems: []string{"Linux"},
			InstanceRegexes:  []*regexp.Regexp{regexp.MustCompile(".*")},
			ClientFactory:    &mockAzureClientFactory{client: azureClient},
		},
	)
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}

	reg := prometheus.NewRegistry()
	err = reg.Register(exp)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	azureFamily := findMetricFamily(families, "azure_pricing_vm")
	if azureFamily == nil {
		t.Fatal("expected azure_pricing_vm metric family")
	}
	if !hasLabelValue(azureFamily, "instance_type", "Standard_D2s_v5") {
		t.Error("expected instance_type=Standard_D2s_v5 on azure_pricing_vm")
	}
}
