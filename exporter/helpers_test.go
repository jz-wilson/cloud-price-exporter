package exporter

import "github.com/pixelfederation/cloud-price-exporter/exporter/aws"

// newTestInstanceStore creates a pre-populated InstanceStore for testing.
// This uses the exported NewInstanceStoreFromMap constructor.
func newTestInstanceStore() *aws.InstanceStore {
	return aws.NewInstanceStoreFromMap(map[string]aws.Instance{
		"m5.large":  {Memory: 8192, VCpu: 2},
		"m5.xlarge": {Memory: 16384, VCpu: 4},
	})
}
