package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
)

// mockEC2Client implements EC2Client for testing.
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

// testInstanceStore creates an InstanceStore pre-populated with test data.
func testInstanceStore() *InstanceStore {
	return &InstanceStore{
		instances: map[string]Instance{
			"m5.large":  {Memory: 8192, VCpu: 2},
			"m5.xlarge": {Memory: 16384, VCpu: 4},
		},
	}
}
