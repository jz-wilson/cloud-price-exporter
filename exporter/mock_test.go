package exporter

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	"github.com/prometheus/client_golang/prometheus"
)

// mockEC2Client implements EC2Client for testing.
type mockEC2Client struct {
	DescribeSpotPriceHistoryFn   func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error)
	DescribeInstanceTypesFn      func(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
	DescribeAvailabilityZonesFn  func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
}

func (m *mockEC2Client) DescribeSpotPriceHistory(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	return m.DescribeSpotPriceHistoryFn(ctx, params, optFns...)
}

func (m *mockEC2Client) DescribeInstanceTypes(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	return m.DescribeInstanceTypesFn(ctx, params, optFns...)
}

func (m *mockEC2Client) DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return m.DescribeAvailabilityZonesFn(ctx, params, optFns...)
}

// mockPricingClient implements pricing.GetProductsAPIClient for testing.
type mockPricingClient struct {
	GetProductsFn func(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

func (m *mockPricingClient) GetProducts(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	return m.GetProductsFn(ctx, params, optFns...)
}

// mockSavingsPlansClient implements SavingsPlansAPI for testing.
type mockSavingsPlansClient struct {
	DescribeSavingsPlansOfferingRatesFn func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error)
}

func (m *mockSavingsPlansClient) DescribeSavingsPlansOfferingRates(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
	return m.DescribeSavingsPlansOfferingRatesFn(ctx, params, optFns...)
}

// mockClientFactory implements ClientFactory for testing.
type mockClientFactory struct {
	ec2Client     EC2Client
	ec2Err        error
	pricingClient pricing.GetProductsAPIClient
	pricingErr    error
	spClient      SavingsPlansAPI
	spErr         error
}

func (f *mockClientFactory) NewEC2Client(region string) (EC2Client, error) {
	return f.ec2Client, f.ec2Err
}

func (f *mockClientFactory) NewPricingClient() (pricing.GetProductsAPIClient, error) {
	return f.pricingClient, f.pricingErr
}

func (f *mockClientFactory) NewSavingsPlansClient() (SavingsPlansAPI, error) {
	return f.spClient, f.spErr
}

// newTestExporter creates an Exporter with pre-populated instances for testing,
// bypassing the NewExporter constructor (which calls getInstances via the factory).
func newTestExporter(factory ClientFactory, opts ...func(*Exporter)) *Exporter {
	e := &Exporter{
		productDescriptions: []string{"Linux/UNIX"},
		operatingSystems:    []string{"Linux"},
		regions:             []string{"us-east-1"},
		lifecycle:           []string{"spot"},
		cache:               0,
		clientFactory:       factory,
		nextScrape:          time.Now(),
		instances: map[string]Instance{
			"m5.large":  {Memory: 8192, VCpu: 2},
			"m5.xlarge": {Memory: 16384, VCpu: 4},
		},
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "aws_pricing",
			Name:      "scrape_duration_seconds",
			Help:      "The scrape duration.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "aws_pricing",
			Name:      "scrapes_total",
			Help:      "Total AWS autoscaling group scrapes.",
		}),
		scrapeErrors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "aws_pricing",
			Name:      "scrape_error",
			Help:      "The scrape error status.",
		}),
	}
	for _, opt := range opts {
		opt(e)
	}
	e.initGauges()
	return e
}
