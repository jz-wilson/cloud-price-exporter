package exporter

import (
	"context"
	"fmt"
	"regexp"
	"testing"
)

func TestGetAzureOnDemandPricing_SingleRegion(t *testing.T) {
	client := &mockAzureRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
			return []AzureRetailPriceItem{
				{RetailPrice: 0.096, ArmRegionName: "eastus", ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series", MeterName: "D2s v5"},
				{RetailPrice: 0.192, ArmRegionName: "eastus", ArmSkuName: "Standard_D4s_v5", ProductName: "Virtual Machines Dv5 Series", MeterName: "D4s v5"},
			}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.azureEnabled = true
		e.azureRegions = []string{"eastus"}
		e.azureOperatingSystems = []string{"Linux"}
		e.azureInstanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
	})

	scrapes := make(chan scrapeResult, 10)
	go func() {
		e.getAzureOnDemandPricing("eastus", client, scrapes)
		close(scrapes)
	}()

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "azure_vm" {
		t.Errorf("expected name 'azure_vm', got %q", results[0].Name)
	}
	if results[0].InstanceType != "Standard_D2s_v5" {
		t.Errorf("expected instance type 'Standard_D2s_v5', got %q", results[0].InstanceType)
	}
	if results[0].Value != 0.096 {
		t.Errorf("expected price 0.096, got %f", results[0].Value)
	}
	if results[0].InstanceLifecycle != "ondemand" {
		t.Errorf("expected lifecycle 'ondemand', got %q", results[0].InstanceLifecycle)
	}
	if results[0].OperatingSystem != "Linux" {
		t.Errorf("expected OS 'Linux', got %q", results[0].OperatingSystem)
	}
}

func TestGetAzureOnDemandPricing_APIError(t *testing.T) {
	client := &mockAzureRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.azureEnabled = true
		e.azureRegions = []string{"eastus"}
		e.azureOperatingSystems = []string{"Linux"}
		e.azureInstanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.errorCount = 0
	})

	scrapes := make(chan scrapeResult, 10)
	go func() {
		e.getAzureOnDemandPricing("eastus", client, scrapes)
		close(scrapes)
	}()

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results on error, got %d", len(results))
	}
	if e.errorCount == 0 {
		t.Error("expected errorCount to be incremented")
	}
}

func TestGetAzureOnDemandPricing_RegexFilter(t *testing.T) {
	client := &mockAzureRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
			return []AzureRetailPriceItem{
				{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series"},
				{RetailPrice: 0.500, ArmSkuName: "Standard_E8s_v5", ProductName: "Virtual Machines Ev5 Series"},
			}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.azureEnabled = true
		e.azureOperatingSystems = []string{"Linux"}
		e.azureInstanceRegexes = []*regexp.Regexp{regexp.MustCompile(`^Standard_D`)}
	})

	scrapes := make(chan scrapeResult, 10)
	go func() {
		e.getAzureOnDemandPricing("eastus", client, scrapes)
		close(scrapes)
	}()

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (regex filtered), got %d", len(results))
	}
	if results[0].InstanceType != "Standard_D2s_v5" {
		t.Errorf("expected Standard_D2s_v5, got %q", results[0].InstanceType)
	}
}

func TestGetAzureOnDemandPricing_OSFilter(t *testing.T) {
	client := &mockAzureRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
			return []AzureRetailPriceItem{
				{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series"},
				{RetailPrice: 0.200, ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series Windows"},
			}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.azureEnabled = true
		e.azureOperatingSystems = []string{"Linux"} // Only Linux
		e.azureInstanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
	})

	scrapes := make(chan scrapeResult, 10)
	go func() {
		e.getAzureOnDemandPricing("eastus", client, scrapes)
		close(scrapes)
	}()

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (OS filtered), got %d", len(results))
	}
	if results[0].OperatingSystem != "Linux" {
		t.Errorf("expected OS 'Linux', got %q", results[0].OperatingSystem)
	}
}

func TestGetAzureOnDemandPricing_EmptyResponse(t *testing.T) {
	client := &mockAzureRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]AzureRetailPriceItem, error) {
			return []AzureRetailPriceItem{}, nil
		},
	}

	e := newTestExporter(nil, func(e *Exporter) {
		e.azureEnabled = true
		e.azureOperatingSystems = []string{"Linux"}
		e.azureInstanceRegexes = []*regexp.Regexp{regexp.MustCompile(".*")}
		e.errorCount = 0
	})

	scrapes := make(chan scrapeResult, 10)
	go func() {
		e.getAzureOnDemandPricing("eastus", client, scrapes)
		close(scrapes)
	}()

	var results []scrapeResult
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if e.errorCount != 0 {
		t.Errorf("expected 0 errors, got %d", e.errorCount)
	}
}

func TestClassifyAzureOS(t *testing.T) {
	tests := []struct {
		productName string
		expected    string
	}{
		{"Virtual Machines Dv5 Series", "Linux"},
		{"Virtual Machines Dv5 Series Windows", "Windows"},
		{"Virtual Machines BS Series Windows", "Windows"},
		{"Virtual Machines Ev5 Series", "Linux"},
		{"Windows Virtual Machines", "Windows"},
	}

	for _, tt := range tests {
		got := classifyAzureOS(tt.productName)
		if got != tt.expected {
			t.Errorf("classifyAzureOS(%q) = %q, want %q", tt.productName, got, tt.expected)
		}
	}
}
