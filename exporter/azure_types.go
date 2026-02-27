package exporter

// AzureRetailPriceResponse represents the response from the Azure Retail Prices API.
type AzureRetailPriceResponse struct {
	Items        []AzureRetailPriceItem `json:"Items"`
	NextPageLink string                 `json:"NextPageLink"`
	Count        int                    `json:"Count"`
}

// AzureRetailPriceItem represents a single pricing item from the Azure Retail Prices API.
type AzureRetailPriceItem struct {
	RetailPrice          float64 `json:"retailPrice"`
	ArmRegionName        string  `json:"armRegionName"`
	ArmSkuName           string  `json:"armSkuName"`
	ProductName          string  `json:"productName"`
	MeterName            string  `json:"meterName"`
	UnitOfMeasure        string  `json:"unitOfMeasure"`
	Type                 string  `json:"type"`
	IsPrimaryMeterRegion bool    `json:"isPrimaryMeterRegion"`
	ServiceName          string  `json:"serviceName"`
	CurrencyCode         string  `json:"currencyCode"`
}
