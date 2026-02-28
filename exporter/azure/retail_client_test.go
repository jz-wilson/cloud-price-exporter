package azure

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClient_SinglePage(t *testing.T) {
	resp := RetailPriceResponse{
		Items: []RetailPriceItem{
			{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", ProductName: "Virtual Machines Dv5 Series", MeterName: "D2s v5", UnitOfMeasure: "1 Hour", ServiceName: "Virtual Machines", IsPrimaryMeterRegion: true},
		},
		Count: 1,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &HTTPRetailPricesClient{
		client:     srv.Client(),
		baseURL:    srv.URL,
		retryDelay: time.Millisecond,
	}

	items, err := client.GetVMPrices(context.Background(), "eastus", []string{"Linux"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ArmSkuName != "Standard_D2s_v5" {
		t.Errorf("expected Standard_D2s_v5, got %q", items[0].ArmSkuName)
	}
}

func TestHTTPClient_Pagination(t *testing.T) {
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			resp := RetailPriceResponse{
				Items: []RetailPriceItem{
					{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", MeterName: "D2s v5", UnitOfMeasure: "1 Hour"},
				},
				NextPageLink: "http://" + r.Host + "/page2",
				Count:        1,
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			resp := RetailPriceResponse{
				Items: []RetailPriceItem{
					{RetailPrice: 0.192, ArmSkuName: "Standard_D4s_v5", MeterName: "D4s v5", UnitOfMeasure: "1 Hour"},
				},
				Count: 1,
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	client := &HTTPRetailPricesClient{
		client:     srv.Client(),
		baseURL:    srv.URL,
		retryDelay: time.Millisecond,
	}

	items, err := client.GetVMPrices(context.Background(), "eastus", []string{"Linux"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items from pagination, got %d", len(items))
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
}

func TestHTTPClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &HTTPRetailPricesClient{
		client:     srv.Client(),
		baseURL:    srv.URL,
		retryDelay: time.Millisecond,
	}

	_, err := client.GetVMPrices(context.Background(), "eastus", []string{"Linux"})
	if err == nil {
		t.Fatal("expected error on 500 status")
	}
}

func TestHTTPClient_FiltersSpotAndLowPriority(t *testing.T) {
	resp := RetailPriceResponse{
		Items: []RetailPriceItem{
			{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", MeterName: "D2s v5", UnitOfMeasure: "1 Hour"},
			{RetailPrice: 0.020, ArmSkuName: "Standard_D2s_v5", MeterName: "D2s v5 Spot", UnitOfMeasure: "1 Hour"},
			{RetailPrice: 0.030, ArmSkuName: "Standard_D2s_v5", MeterName: "D2s v5 Low Priority", UnitOfMeasure: "1 Hour"},
		},
		Count: 3,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &HTTPRetailPricesClient{
		client:     srv.Client(),
		baseURL:    srv.URL,
		retryDelay: time.Millisecond,
	}

	items, err := client.GetVMPrices(context.Background(), "eastus", []string{"Linux"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (Spot and Low Priority filtered), got %d", len(items))
	}
	if items[0].RetailPrice != 0.096 {
		t.Errorf("expected price 0.096, got %f", items[0].RetailPrice)
	}
}

func TestHTTPClient_RetriesOnTransientError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp := RetailPriceResponse{
			Items: []RetailPriceItem{
				{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", MeterName: "D2s v5", UnitOfMeasure: "1 Hour"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &HTTPRetailPricesClient{
		client:     srv.Client(),
		baseURL:    srv.URL,
		retryDelay: time.Millisecond,
	}

	items, err := client.GetVMPrices(context.Background(), "eastus", []string{"Linux"})
	if err != nil {
		t.Fatalf("expected retry to succeed on third attempt, got error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after successful retry, got %d", len(items))
	}
	if callCount != 3 {
		t.Errorf("expected 3 HTTP calls (2 failures + 1 success), got %d", callCount)
	}
}

func TestHTTPClient_ExhaustsAllRetries(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &HTTPRetailPricesClient{
		client:     srv.Client(),
		baseURL:    srv.URL,
		retryDelay: time.Millisecond,
	}

	_, err := client.GetVMPrices(context.Background(), "eastus", []string{"Linux"})
	if err == nil {
		t.Fatal("expected error after exhausting all retries")
	}
	if callCount != 3 {
		t.Errorf("expected exactly 3 attempts (maxRetries), got %d", callCount)
	}
}

func TestHTTPClient_FiltersNonHourly(t *testing.T) {
	resp := RetailPriceResponse{
		Items: []RetailPriceItem{
			{RetailPrice: 0.096, ArmSkuName: "Standard_D2s_v5", MeterName: "D2s v5", UnitOfMeasure: "1 Hour"},
			{RetailPrice: 70.08, ArmSkuName: "Standard_D2s_v5", MeterName: "D2s v5", UnitOfMeasure: "1 Month"},
		},
		Count: 2,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &HTTPRetailPricesClient{
		client:     srv.Client(),
		baseURL:    srv.URL,
		retryDelay: time.Millisecond,
	}

	items, err := client.GetVMPrices(context.Background(), "eastus", []string{"Linux"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (non-hourly filtered), got %d", len(items))
	}
}
