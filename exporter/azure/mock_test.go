package azure

import (
	"context"
	"testing"

	"github.com/jz-wilson/cloud-price-exporter/exporter/provider"
)

// drainScrapes collects all results from a closed ScrapeResult channel.
func drainScrapes(t *testing.T, ch <-chan provider.ScrapeResult) []provider.ScrapeResult {
	t.Helper()
	var results []provider.ScrapeResult
	for r := range ch {
		results = append(results, r)
	}
	return results
}

// requireScrapeCount fatally fails if the result count doesn't match expected.
func requireScrapeCount(t *testing.T, results []provider.ScrapeResult, want int) {
	t.Helper()
	if len(results) != want {
		t.Fatalf("expected %d scrape results, got %d", want, len(results))
	}
}

// mockRetailPricesClient implements RetailPricesClient for testing.
type mockRetailPricesClient struct {
	GetVMPricesFn func(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error)
}

func (m *mockRetailPricesClient) GetVMPrices(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error) {
	if m.GetVMPricesFn != nil {
		return m.GetVMPricesFn(ctx, region, osTypes)
	}
	return nil, nil
}
