package exporter

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	"github.com/prometheus/client_golang/prometheus"
)

func newMockFactoryWithInstances() *mockClientFactory {
	return &mockClientFactory{
		ec2Client: &mockEC2Client{
			DescribeInstanceTypesFn: func(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
				return &ec2.DescribeInstanceTypesOutput{
					InstanceTypes: []ec2types.InstanceTypeInfo{
						{
							InstanceType: ec2types.InstanceTypeM5Large,
							MemoryInfo:   &ec2types.MemoryInfo{SizeInMiB: aws.Int64(8192)},
							VCpuInfo:     &ec2types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
						},
					},
				}, nil
			},
			DescribeSpotPriceHistoryFn: func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
				return &ec2.DescribeSpotPriceHistoryOutput{
					SpotPriceHistory: []ec2types.SpotPrice{
						{
							InstanceType:       ec2types.InstanceTypeM5Large,
							SpotPrice:          aws.String("0.05"),
							AvailabilityZone:   aws.String("us-east-1a"),
							ProductDescription: ec2types.RIProductDescriptionLinuxUnix,
						},
					},
				}, nil
			},
			DescribeAvailabilityZonesFn: func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
				return &ec2.DescribeAvailabilityZonesOutput{
					AvailabilityZones: []ec2types.AvailabilityZone{
						{ZoneName: aws.String("us-east-1a")},
					},
				}, nil
			},
		},
		pricingClient: &mockPricingClient{
			GetProductsFn: func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
				return &pricing.GetProductsOutput{}, nil
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
	if len(exp.instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(exp.instances))
	}
}

func TestNewExporter_FailsOnInstancesError(t *testing.T) {
	factory := &mockClientFactory{
		ec2Client: &mockEC2Client{
			DescribeInstanceTypesFn: func(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
				return nil, fmt.Errorf("no permissions")
			},
		},
	}

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
		t.Fatal("expected error from NewExporter when getInstances fails")
	}
}

func TestNewExporter_FailsOnEC2ClientCreation(t *testing.T) {
	factory := &mockClientFactory{
		ec2Err: fmt.Errorf("failed to load config"),
	}

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
		t.Fatal("expected error when EC2 client creation fails")
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

	var descs []*prometheus.Desc
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
		e.cache = 3600 // 1 hour cache
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
					SpotPrice:          aws.String("0.05"),
					AvailabilityZone:   aws.String("us-east-1a"),
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

	scrapes := make(chan scrapeResult, 10)
	scrapes <- scrapeResult{
		Name:              "ec2",
		Value:             0.05,
		Region:            "us-east-1",
		AvailabilityZone:  "us-east-1a",
		InstanceType:      "m5.large",
		InstanceLifecycle: "spot",
		Memory:            "8192",
		VCpu:              "2",
	}
	scrapes <- scrapeResult{
		Name:              "ec2_memory",
		Value:             0.002,
		Region:            "us-east-1",
		AvailabilityZone:  "us-east-1a",
		InstanceType:      "m5.large",
		InstanceLifecycle: "spot",
	}
	scrapes <- scrapeResult{
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
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Error("expected true for 'b' in [a,b,c]")
	}
	if contains([]string{"a", "b", "c"}, "d") {
		t.Error("expected false for 'd' in [a,b,c]")
	}
	if contains([]string{}, "a") {
		t.Error("expected false for empty slice")
	}
}

func TestIsMatchAny(t *testing.T) {
	regexes := []*regexp.Regexp{
		regexp.MustCompile(`^m5\.`),
		regexp.MustCompile(`^c5\.`),
	}

	if !isMatchAny(regexes, "m5.large") {
		t.Error("expected match for m5.large")
	}
	if !isMatchAny(regexes, "c5.xlarge") {
		t.Error("expected match for c5.xlarge")
	}
	if isMatchAny(regexes, "r5.large") {
		t.Error("expected no match for r5.large")
	}
	if isMatchAny(nil, "anything") {
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

	var descs []*prometheus.Desc
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
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
			return []AzureRetailPriceItem{
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
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
			return []AzureRetailPriceItem{
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
