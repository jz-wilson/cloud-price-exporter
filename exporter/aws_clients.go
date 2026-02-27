package exporter

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
)

// AWSClientFactory creates real AWS SDK clients. Implements ClientFactory.
type AWSClientFactory struct{}

func (f *AWSClientFactory) NewEC2Client(region string) (EC2Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for EC2 [region=%s]: %w", region, err)
	}
	return ec2.NewFromConfig(cfg), nil
}

func (f *AWSClientFactory) NewPricingClient() (pricing.GetProductsAPIClient, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for Pricing API: %w", err)
	}
	return pricing.NewFromConfig(cfg), nil
}

func (f *AWSClientFactory) NewSavingsPlansClient() (SavingsPlansAPI, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for SavingsPlans API: %w", err)
	}
	return savingsplans.NewFromConfig(cfg), nil
}
