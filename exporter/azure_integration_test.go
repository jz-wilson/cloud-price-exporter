//go:build integration

package exporter

import (
	"context"
	"testing"
	"time"
)

func TestIntegration_AzureVMPricing(t *testing.T) {
	factory := &DefaultAzureClientFactory{}
	client := factory.NewAzureRetailPricesClient()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	items, err := client.GetVMPrices(ctx, "eastus", []string{"Linux"})
	if err != nil {
		t.Fatalf("Azure API call failed: %v", err)
	}

	if len(items) == 0 {
		t.Fatal("expected at least some VM pricing items from Azure API")
	}

	// Verify we can find a well-known VM size
	found := false
	for _, item := range items {
		if item.ArmSkuName == "Standard_D2s_v5" {
			found = true
			if item.RetailPrice <= 0 {
				t.Errorf("expected positive price for Standard_D2s_v5, got %f", item.RetailPrice)
			}
			t.Logf("Standard_D2s_v5 price: $%.4f/hr", item.RetailPrice)
			break
		}
	}
	if !found {
		t.Log("Standard_D2s_v5 not found, checking for any Standard_D series...")
		for _, item := range items {
			if len(item.ArmSkuName) > 10 && item.ArmSkuName[:10] == "Standard_D" {
				t.Logf("Found %s at $%.4f/hr", item.ArmSkuName, item.RetailPrice)
				found = true
				break
			}
		}
		if !found {
			t.Error("could not find any Standard_D series VM in results")
		}
	}

	t.Logf("Total VM pricing items returned: %d", len(items))
}
