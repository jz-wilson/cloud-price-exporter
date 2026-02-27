package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
)

// NOTE: ec2.DescribeInstanceTypesAPIClient was removed from EC2Client because
// instance type data is now fetched from ec2instances.info via HTTP, not the
// AWS DescribeInstanceTypes API.

// EC2DescribeAZsAPI wraps the DescribeAvailabilityZones call (no SDK paginator interface exists).
type EC2DescribeAZsAPI interface {
	DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
}

// SavingsPlansAPI wraps the DescribeSavingsPlansOfferingRates call (no SDK paginator interface exists).
type SavingsPlansAPI interface {
	DescribeSavingsPlansOfferingRates(ctx context.Context, params *savingsplans.DescribeSavingsPlansOfferingRatesInput, optFns ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOfferingRatesOutput, error)
}

// EC2Client combines the EC2 API interfaces needed by this exporter.
type EC2Client interface {
	ec2.DescribeSpotPriceHistoryAPIClient
	EC2DescribeAZsAPI
}

// ClientFactory creates AWS service clients, enabling dependency injection for testing.
type ClientFactory interface {
	NewEC2Client(region string) (EC2Client, error)
	NewPricingClient() (pricing.GetProductsAPIClient, error)
	NewSavingsPlansClient() (SavingsPlansAPI, error)
}
