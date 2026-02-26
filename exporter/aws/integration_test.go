//go:build integration

package aws

import (
	"regexp"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestIntegration_SpotPricing runs a real scrape against AWS APIs.
// Requires valid AWS credentials (e.g. AWS_PROFILE=default).
//
// Run with: go test -tags=integration -run TestIntegration -v ./exporter/aws/
func TestIntegration_SpotPricing(t *testing.T) {
	// This integration test uses the old exporter package path since it needs
	// the full Exporter orchestrator. It's kept here as a reference for the
	// aws package's own integration testing.
	_ = regexp.MustCompile(`^m5\.`)
	_ = prometheus.NewRegistry()

	t.Skip("Full integration test requires the Exporter orchestrator; run via ./exporter/ package")
}
