package aws

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestInstanceStore_Load_Success(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"instance_type": "m5.large",  "vcpu": 2, "memory": 8.0},
			{"instance_type": "m5.xlarge", "vcpu": 4, "memory": 16.0}
		]`))
	})
	defer ts.Close()

	store := NewInstanceStore()
	store.url = ts.URL
	err := store.Load(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.Len() != 2 {
		t.Fatalf("expected 2 instances, got %d", store.Len())
	}

	if got := store.GetMemory("m5.large"); got != "8192" {
		t.Errorf("m5.large memory: expected 8192, got %s", got)
	}
	if got := store.GetVCpu("m5.large"); got != "2" {
		t.Errorf("m5.large vcpu: expected 2, got %s", got)
	}
	if got := store.GetMemory("m5.xlarge"); got != "16384" {
		t.Errorf("m5.xlarge memory: expected 16384, got %s", got)
	}
	if got := store.GetVCpu("m5.xlarge"); got != "4" {
		t.Errorf("m5.xlarge vcpu: expected 4, got %s", got)
	}
}

func TestInstanceStore_Load_HTTPError(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer ts.Close()

	store := NewInstanceStore()
	store.url = ts.URL
	err := store.Load(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error from Load on HTTP 500, got nil")
	}
}

func TestInstanceStore_Load_BadJSON(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json`))
	})
	defer ts.Close()

	store := NewInstanceStore()
	store.url = ts.URL
	err := store.Load(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error from Load on bad JSON, got nil")
	}
}

func TestInstanceStore_Load_EmptyResponse(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})
	defer ts.Close()

	store := NewInstanceStore()
	store.url = ts.URL
	err := store.Load(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.Len() != 0 {
		t.Errorf("expected 0 instances, got %d", store.Len())
	}
}

func TestInstanceStore_Load_CustomHTTPClient(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"instance_type": "t3.micro", "vcpu": 2, "memory": 1.0}]`))
	})
	defer ts.Close()

	store := NewInstanceStore()
	store.url = ts.URL
	err := store.Load(context.Background(), ts.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.Len() != 1 {
		t.Fatalf("expected 1 instance, got %d", store.Len())
	}
	if got := store.GetMemory("t3.micro"); got != "1024" {
		t.Errorf("t3.micro memory: expected 1024, got %s", got)
	}
}

func TestInstanceStore_GetNormalizedCost(t *testing.T) {
	store := testInstanceStore()

	// m5.large: 2 vCPUs, 8192 MiB = 8 GiB
	// memoryCost = 0.096 / (7.2*2 + 8) = 0.096 / 22.4
	// vcpuCost = 7.2 * memoryCost
	vcpu, memory := store.GetNormalizedCost(0.096, "m5.large")

	expectedMemory := 0.096 / (7.2*2 + 8)
	expectedVCpu := 7.2 * expectedMemory

	if math.Abs(vcpu-expectedVCpu) > 1e-10 {
		t.Errorf("vcpu cost: expected %v, got %v", expectedVCpu, vcpu)
	}
	if math.Abs(memory-expectedMemory) > 1e-10 {
		t.Errorf("memory cost: expected %v, got %v", expectedMemory, memory)
	}
}

func TestInstanceStore_GetNormalizedCost_UnknownInstance(t *testing.T) {
	store := NewInstanceStore()

	vcpu, memory := store.GetNormalizedCost(0.1, "unknown.type")

	if vcpu != 0 {
		t.Errorf("expected 0 for unknown instance vcpu, got %v", vcpu)
	}
	if memory != 0 {
		t.Errorf("expected 0 for unknown instance memory, got %v", memory)
	}
}

func TestInstanceStore_GetMemory(t *testing.T) {
	store := testInstanceStore()

	if got := store.GetMemory("m5.large"); got != "8192" {
		t.Errorf("expected 8192, got %s", got)
	}

	// Unknown instance returns "0"
	if got := store.GetMemory("unknown"); got != "0" {
		t.Errorf("expected 0 for unknown instance, got %s", got)
	}
}

func TestInstanceStore_GetVCpu(t *testing.T) {
	store := testInstanceStore()

	if got := store.GetVCpu("m5.large"); got != "2" {
		t.Errorf("expected 2, got %s", got)
	}

	if got := store.GetVCpu("unknown"); got != "0" {
		t.Errorf("expected 0 for unknown instance, got %s", got)
	}
}
