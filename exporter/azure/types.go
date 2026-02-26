package azure

// RetailPriceResponse represents the response from the Azure Retail Prices API.
type RetailPriceResponse struct {
	Items        []RetailPriceItem `json:"Items"`
	NextPageLink string            `json:"NextPageLink"`
	Count        int               `json:"Count"`
}

// RetailPriceItem represents a single pricing item from the Azure Retail Prices API.
type RetailPriceItem struct {
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
