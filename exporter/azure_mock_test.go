package exporter

import "context"

// mockAzureRetailPricesClient implements AzureRetailPricesClient for testing.
type mockAzureRetailPricesClient struct {
	GetVMPricesFn func(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error)
}

func (m *mockAzureRetailPricesClient) GetVMPrices(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
	if m.GetVMPricesFn != nil {
		return m.GetVMPricesFn(ctx, region, osTypes)
	}
	return nil, nil
}

// mockAzureClientFactory implements AzureClientFactory for testing.
type mockAzureClientFactory struct {
	client AzureRetailPricesClient
}

func (f *mockAzureClientFactory) NewAzureRetailPricesClient() AzureRetailPricesClient {
	return f.client
}
