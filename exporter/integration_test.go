//go:build integration

package exporter

import (
	"regexp"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jz-wilson/cloud-price-exporter/exporter/aws"
)

// TestIntegration_SpotPricing runs a real scrape against AWS APIs.
// Requires valid AWS credentials (e.g. AWS_PROFILE=default).
//
// Run with: go test -tags=integration -run TestIntegration -v ./exporter/
func TestIntegration_SpotPricing(t *testing.T) {
	factory := &aws.SDKClientFactory{}

	exp, err := NewExporter(
		[]string{"Linux/UNIX"},
		[]string{"Linux"},
		[]string{"us-east-1"},
		[]string{"spot"},
		0,
		[]*regexp.Regexp{regexp.MustCompile(`^m5\.`)},
		[]string{},
		factory,
		nil,
	)
	if err != nil {
		t.Fatalf("NewExporter failed: %v", err)
	}

	// Verify it satisfies the Prometheus collector interface
	reg := prometheus.NewRegistry()
	if err := reg.Register(exp); err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Should have at least the 3 internal metrics + some pricing data
	if len(metrics) < 3 {
		t.Errorf("expected at least 3 metric families, got %d", len(metrics))
	}

	// Check for the main ec2 pricing metric
	found := false
	for _, m := range metrics {
		t.Logf("metric: %s (%d series)", m.GetName(), len(m.GetMetric()))
		if m.GetName() == "aws_pricing_ec2" {
			found = true
			if len(m.GetMetric()) == 0 {
				t.Error("aws_pricing_ec2 has 0 series — expected spot prices")
			}
		}
	}
	if !found {
		t.Error("aws_pricing_ec2 metric not found in output")
	}
}
