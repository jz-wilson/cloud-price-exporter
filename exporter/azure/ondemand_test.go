package azure

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/jz-wilson/cloud-price-exporter/exporter/provider"
)

func TestGetOnDemandPricing_SingleRegion(t *testing.T) {
	client := &mockRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error) {
			return []RetailPriceItem{
				{RetailPrice: 0.096, ArmRegionName: "eastus", ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series", MeterName: "D2s v5"},
				{RetailPrice: 0.192, ArmRegionName: "eastus", ArmSkuName: "Standard_D4s_v5", ProductName: "Virtual Machines Dv5 Series", MeterName: "D4s v5"},
			}, nil
		},
	}

	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 10)
	go func() {
		GetOnDemandPricing(context.Background(), "eastus", client, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, &errorCount, scrapes)
		close(scrapes)
	}()

	results := make([]provider.ScrapeResult, 0, len(scrapes))
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

func TestGetOnDemandPricing_APIError(t *testing.T) {
	client := &mockRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 10)
	go func() {
		GetOnDemandPricing(context.Background(), "eastus", client, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, &errorCount, scrapes)
		close(scrapes)
	}()

	results := make([]provider.ScrapeResult, 0, len(scrapes))
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results on error, got %d", len(results))
	}
	if errorCount == 0 {
		t.Error("expected errorCount to be incremented")
	}
}

func TestGetOnDemandPricing_RegexFilter(t *testing.T) {
	client := &mockRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error) {
			return []RetailPriceItem{
				{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series"},
				{RetailPrice: 0.500, ArmSkuName: "Standard_E8s_v5", ProductName: "Virtual Machines Ev5 Series"},
			}, nil
		},
	}

	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 10)
	go func() {
		GetOnDemandPricing(context.Background(), "eastus", client, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(`^Standard_D`)}, &errorCount, scrapes)
		close(scrapes)
	}()

	results := make([]provider.ScrapeResult, 0, len(scrapes))
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

func TestGetOnDemandPricing_OSFilter(t *testing.T) {
	client := &mockRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error) {
			return []RetailPriceItem{
				{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series"},
				{RetailPrice: 0.200, ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series Windows"},
			}, nil
		},
	}

	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 10)
	go func() {
		GetOnDemandPricing(context.Background(), "eastus", client, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, &errorCount, scrapes)
		close(scrapes)
	}()

	results := make([]provider.ScrapeResult, 0, len(scrapes))
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

func TestGetOnDemandPricing_EmptyResponse(t *testing.T) {
	client := &mockRetailPricesClient{
		GetVMPricesFn: func(ctx context.Context, region string, osTypes []string) ([]RetailPriceItem, error) {
			return []RetailPriceItem{}, nil
		},
	}

	var errorCount uint64
	scrapes := make(chan provider.ScrapeResult, 10)
	go func() {
		GetOnDemandPricing(context.Background(), "eastus", client, []string{"Linux"}, []*regexp.Regexp{regexp.MustCompile(".*")}, &errorCount, scrapes)
		close(scrapes)
	}()

	results := make([]provider.ScrapeResult, 0, len(scrapes))
	for r := range scrapes {
		results = append(results, r)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if errorCount != 0 {
		t.Errorf("expected 0 errors, got %d", errorCount)
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
