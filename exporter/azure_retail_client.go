package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const azureRetailPricesBaseURL = "https://prices.azure.com/api/retail/prices"

// DefaultAzureClientFactory creates production Azure API clients.
type DefaultAzureClientFactory struct{}

func (f *DefaultAzureClientFactory) NewAzureRetailPricesClient() AzureRetailPricesClient {
	return &HTTPAzureRetailPricesClient{
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: azureRetailPricesBaseURL,
	}
}

// HTTPAzureRetailPricesClient calls the Azure Retail Prices REST API over HTTP.
type HTTPAzureRetailPricesClient struct {
	client  *http.Client
	baseURL string // overridable for tests
}

func (c *HTTPAzureRetailPricesClient) GetVMPrices(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
	filter := fmt.Sprintf(
		"serviceName eq 'Virtual Machines' and priceType eq 'Consumption' and armRegionName eq '%s' and isPrimaryMeterRegion eq true",
		region,
	)

	// Apply OS-level filtering at the API to reduce data transfer.
	// Azure has no explicit "os" field — Windows products contain "Windows" in productName.
	// Only applied for single-OS configs; multi-OS falls back to client-side filtering in azure_ondemand.go.
	if len(osTypes) == 1 {
		switch osTypes[0] {
		case "Windows":
			filter += " and contains(productName, 'Windows')"
		case "Linux":
			filter += " and not contains(productName, 'Windows')"
		}
	}

	nextURL := fmt.Sprintf("%s?$filter=%s", c.baseURL, url.QueryEscape(filter))

	var results []AzureRetailPriceItem

	for nextURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching Azure prices: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("Azure API returned status %d", resp.StatusCode)
		}

		var page AzureRetailPriceResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding Azure response: %w", err)
		}
		resp.Body.Close()

		for _, item := range page.Items {
			if item.UnitOfMeasure != "1 Hour" {
				continue
			}
			if strings.Contains(item.MeterName, "Spot") || strings.Contains(item.MeterName, "Low Priority") {
				continue
			}
			results = append(results, item)
		}

		nextURL = page.NextPageLink
	}

	return results, nil
}
