package exporter

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pixelfederation/cloud-price-exporter/exporter/aws"
	"github.com/pixelfederation/cloud-price-exporter/exporter/azure"
)

// mockEC2Client implements aws.EC2Client for testing.
type mockEC2Client struct {
	DescribeSpotPriceHistoryFn  func(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error)
	DescribeAvailabilityZonesFn func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
}

func (m *mockEC2Client) DescribeSpotPriceHistory(ctx context.Context, params *ec2.DescribeSpotPriceHistoryInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	return m.DescribeSpotPriceHistoryFn(ctx, params, optFns...)
}

func (m *mockEC2Client) DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return m.DescribeAvailabilityZonesFn(ctx, params, optFns...)
}

// mockSavingsPlansClient implements aws.SavingsPlansAPI for testing.
type mockSavingsPlansClient struct {
	DescribeSavingsPlansOfferingRatesFn func(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error)
}

func (m *mockSavingsPlansClient) DescribeSavingsPlansOfferingRates(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error) {
	return m.DescribeSavingsPlansOfferingRatesFn(ctx, params, optFns...)
}

// mockClientFactory implements aws.ClientFactory for testing.
type mockClientFactory struct {
	ec2Client aws.EC2Client
	ec2Err    error
	spClient  aws.SavingsPlansAPI
	spErr     error
}

func (f *mockClientFactory) NewEC2Client(region string) (aws.EC2Client, error) {
	return f.ec2Client, f.ec2Err
}

func (f *mockClientFactory) NewSavingsPlansClient() (aws.SavingsPlansAPI, error) {
	return f.spClient, f.spErr
}

// mockAzureRetailPricesClient implements azure.RetailPricesClient for testing.
type mockAzureRetailPricesClient struct {
	GetVMPricesFn func(ctx context.Context, region string, osTypes []string) ([]azure.RetailPriceItem, error)
}

func (m *mockAzureRetailPricesClient) GetVMPrices(ctx context.Context, region string, osTypes []string) ([]azure.RetailPriceItem, error) {
	if m.GetVMPricesFn != nil {
		return m.GetVMPricesFn(ctx, region, osTypes)
	}
	return nil, nil
}

// mockAzureClientFactory implements azure.ClientFactory for testing.
type mockAzureClientFactory struct {
	client azure.RetailPricesClient
}

func (f *mockAzureClientFactory) NewRetailPricesClient() azure.RetailPricesClient {
	return f.client
}

// newTestExporter creates an Exporter with pre-populated instances for testing,
// bypassing the NewExporter constructor (which calls InstanceStore.Load via the factory).
func newTestExporter(factory aws.ClientFactory, opts ...func(*Exporter)) *Exporter {
	e := &Exporter{
		productDescriptions: []string{"Linux/UNIX"},
		operatingSystems:    []string{"Linux"},
		regions:             []string{"us-east-1"},
		lifecycle:           []string{"spot"},
		cache:               0,
		clientFactory:       factory,
		instances:           newTestInstanceStore(),
		nextScrape:          time.Now(),
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
