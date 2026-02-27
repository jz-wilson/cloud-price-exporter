package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
)

// SDKClientFactory creates real AWS SDK clients. Implements ClientFactory.
type SDKClientFactory struct{}

func (f *SDKClientFactory) NewEC2Client(region string) (EC2Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for EC2 [region=%s]: %w", region, err)
	}
	return ec2.NewFromConfig(cfg), nil
}

func (f *SDKClientFactory) NewSavingsPlansClient() (SavingsPlansAPI, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for SavingsPlans API: %w", err)
	}
	return savingsplans.NewFromConfig(cfg), nil
}
