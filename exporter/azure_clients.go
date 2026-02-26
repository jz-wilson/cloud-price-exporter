package exporter

import "context"

// AzureRetailPricesClient fetches VM pricing from the Azure Retail Prices API.
type AzureRetailPricesClient interface {
	GetVMPrices(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error)
}

// AzureClientFactory creates Azure API clients, enabling dependency injection for testing.
type AzureClientFactory interface {
	NewAzureRetailPricesClient() AzureRetailPricesClient
}
