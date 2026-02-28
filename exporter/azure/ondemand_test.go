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
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 2)
	for i, r := range results {
		if r.Name != "azure_vm" {
			t.Errorf("results[%d]: expected name=azure_vm, got %q", i, r.Name)
		}
		if r.InstanceLifecycle != "ondemand" {
			t.Errorf("results[%d]: expected lifecycle=ondemand, got %q", i, r.InstanceLifecycle)
		}
		if r.OperatingSystem != "Linux" {
			t.Errorf("results[%d]: expected OS=Linux, got %q", i, r.OperatingSystem)
		}
		if r.Region != "eastus" {
			t.Errorf("results[%d]: expected region=eastus, got %q", i, r.Region)
		}
	}
	if results[0].InstanceType != "Standard_D2s_v5" || results[0].Value != 0.096 {
		t.Errorf("first result: want Standard_D2s_v5/0.096, got %s/%v", results[0].InstanceType, results[0].Value)
	}
	if results[1].InstanceType != "Standard_D4s_v5" || results[1].Value != 0.192 {
		t.Errorf("second result: want Standard_D4s_v5/0.192, got %s/%v", results[1].InstanceType, results[1].Value)
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
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 0)
	if errorCount == 0 {
		t.Error("expected errorCount to be incremented on API error")
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
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 1) // Standard_E8s_v5 filtered out
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
	results := drainScrapes(t, scrapes)

	requireScrapeCount(t, results, 1) // Windows variant filtered out
	if results[0].OperatingSystem != "Linux" {
		t.Errorf("expected OS=Linux, got %q", results[0].OperatingSystem)
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

	requireScrapeCount(t, drainScrapes(t, scrapes), 0)
	if errorCount != 0 {
		t.Errorf("expected 0 errors for empty response, got %d", errorCount)
	}
}

func TestClassifyAzureOS(t *testing.T) {
	tests := []struct {
		productName string
		want        string
	}{
		{"Virtual Machines Dv5 Series", "Linux"},
		{"Virtual Machines Dv5 Series Windows", "Windows"},
		{"Virtual Machines BS Series Windows", "Windows"},
		{"Virtual Machines Ev5 Series", "Linux"},
		{"Windows Virtual Machines", "Windows"},
		{"", "Linux"}, // empty string has no "Windows" → Linux
	}

	for _, tt := range tests {
		t.Run(tt.productName, func(t *testing.T) {
			got := classifyAzureOS(tt.productName)
			if got != tt.want {
				t.Errorf("classifyAzureOS(%q) = %q, want %q", tt.productName, got, tt.want)
			}
		})
	}
}
