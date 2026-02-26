package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const retailPricesBaseURL = "https://prices.azure.com/api/retail/prices"

// DefaultClientFactory creates production Azure API clients.
// A single shared HTTP client is reused across all regions for connection pooling.
type DefaultClientFactory struct {
	client *http.Client
}

func NewDefaultClientFactory() *DefaultClientFactory {
	return &DefaultClientFactory{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

func (f *DefaultClientFactory) NewRetailPricesClient() RetailPricesClient {
	return &HTTPRetailPricesClient{
		client:     f.client,
		baseURL:    retailPricesBaseURL,
		retryDelay: time.Second,
	}
}

// HTTPRetailPricesClient calls the Azure Retail Prices REST API over HTTP.
type HTTPRetailPricesClient struct {
	client     *http.Client
	baseURL    string        // overridable for tests
	retryDelay time.Duration // base unit for exponential backoff; defaults to time.Second
}

func (c *HTTPRetailPricesClient) GetVMPrices(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error) {
	filter := fmt.Sprintf(
		"serviceName eq 'Virtual Machines' and priceType eq 'Consumption' and armRegionName eq '%s' and isPrimaryMeterRegion eq true",
		region,
	)

	// Apply OS-level filtering at the API to reduce data transfer.
	// Azure has no explicit "os" field — Windows products contain "Windows" in productName.
	// Only applied for single-OS configs; multi-OS falls back to client-side filtering in ondemand.go.
	if len(osTypes) == 1 {
		switch osTypes[0] {
		case "Windows":
			filter += " and contains(productName, 'Windows')"
		case "Linux":
			filter += " and contains(productName, 'Windows') eq false"
		}
	}

	nextURL := fmt.Sprintf("%s?$filter=%s", c.baseURL, url.QueryEscape(filter))

	var results []RetailPriceItem

	for nextURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		resp, err := c.doWithRetry(req)
		if err != nil {
			return nil, fmt.Errorf("fetching Azure prices: %w", err)
		}

		var page RetailPriceResponse
		err = json.NewDecoder(resp.Body).Decode(&page)
		if err != nil {
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
			if item.RetailPrice <= 0 {
				log.Debugf("Skipping Azure item with non-positive price: sku=%s region=%s price=%f", item.ArmSkuName, item.ArmRegionName, item.RetailPrice)
				continue
			}
			if item.ArmSkuName == "" {
				log.Debugf("Skipping Azure item with empty armSkuName: meterName=%s region=%s", item.MeterName, item.ArmRegionName)
				continue
			}
			results = append(results, item)
		}

		validNext, err := validateNextPageLink(page.NextPageLink, c.baseURL)
		if err != nil {
			log.WithError(err).Warn("invalid NextPageLink, stopping pagination")
			break
		}
		nextURL = validNext
	}

	return results, nil
}

const maxRetries = 3

func (c *HTTPRetailPricesClient) doWithRetry(req *http.Request) (*http.Response, error) {
	delay := c.retryDelay
	if delay == 0 {
		delay = time.Second
	}
	var lastErr error
	for attempt := range maxRetries {
		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(1<<attempt) * delay)
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("Azure API returned status %d", resp.StatusCode)
			time.Sleep(time.Duration(1<<attempt) * delay)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("Azure API returned status %d", resp.StatusCode)
		}
		return resp, nil
	}
	return nil, fmt.Errorf("Azure API failed after %d retries: %w", maxRetries, lastErr)
}

func validateNextPageLink(next, baseURL string) (string, error) {
	if next == "" {
		return "", nil
	}
	u, err := url.Parse(next)
	if err != nil {
		return "", fmt.Errorf("invalid NextPageLink %q: %w", next, err)
	}
	base, _ := url.Parse(baseURL)
	if u.Host != base.Host || u.Scheme != base.Scheme {
		return "", fmt.Errorf("NextPageLink host %q does not match expected %q", u.Host, base.Host)
	}
	return next, nil
}
