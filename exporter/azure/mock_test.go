package azure

import "context"

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
