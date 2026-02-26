package azure

import "context"

// RetailPricesClient fetches VM pricing from the Azure Retail Prices API.
type RetailPricesClient interface {
	GetVMPrices(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error)
}

// ClientFactory creates Azure API clients, enabling dependency injection for testing.
type ClientFactory interface {
	NewRetailPricesClient() RetailPricesClient
}
