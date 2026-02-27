package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
)

// EC2InstancesInfoURL is the default URL to fetch EC2 instance type data from.
// Exported so tests in other packages can override it.
var EC2InstancesInfoURL = "https://ec2instances.info/instances.json"

// ec2InstanceInfo represents a single entry from the ec2instances.info JSON API.
type ec2InstanceInfo struct {
	InstanceType string  `json:"instance_type"`
	VCpu         int     `json:"vcpu"`
	Memory       float64 `json:"memory"` // GiB
}

// InstanceStore caches EC2 instance type specifications (vCPU, memory).
type InstanceStore struct {
	instances map[string]Instance
	url       string // override URL for testing; empty = use EC2InstancesInfoURL
}

// NewInstanceStore returns an empty InstanceStore.
func NewInstanceStore() *InstanceStore {
	return &InstanceStore{instances: make(map[string]Instance)}
}

// NewInstanceStoreFromMap creates an InstanceStore pre-populated with the given instances.
func NewInstanceStoreFromMap(instances map[string]Instance) *InstanceStore {
	return &InstanceStore{instances: instances}
}

// Load fetches all instance types from ec2instances.info and populates the store.
// Pass nil for httpClient to use http.DefaultClient.
func (s *InstanceStore) Load(ctx context.Context, httpClient *http.Client) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	url := s.url
	if url == "" {
		url = EC2InstancesInfoURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error creating request for instance data: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching instance data from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d fetching instance data from %s", resp.StatusCode, url)
	}

	var items []ec2InstanceInfo
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return fmt.Errorf("error parsing instance data: %w", err)
	}

	s.instances = make(map[string]Instance, len(items))
	for _, item := range items {
		s.instances[item.InstanceType] = Instance{
			Memory: int64(item.Memory * 1024), // GiB -> MiB
			VCpu:   int32(item.VCpu),
		}
	}

	log.Infof("loaded %d instance types", len(s.instances))
	return nil
}

// Len returns the number of cached instance types.
func (s *InstanceStore) Len() int {
	return len(s.instances)
}

// GetMemory returns the memory (MiB) of the named instance type as a string.
func (s *InstanceStore) GetMemory(instanceType string) string {
	return strconv.Itoa(int(s.instances[instanceType].Memory))
}

// GetVCpu returns the vCPU count of the named instance type as a string.
func (s *InstanceStore) GetVCpu(instanceType string) string {
	return strconv.Itoa(int(s.instances[instanceType].VCpu))
}

// GetNormalizedCost computes per-vCPU and per-GB-memory costs using the
// 7.2 CPU-to-memory ratio. Returns (0, 0) for unknown instances.
func (s *InstanceStore) GetNormalizedCost(value float64, instanceType string) (vcpuCost, memoryCost float64) {
	inst, ok := s.instances[instanceType]
	if !ok {
		return 0, 0
	}
	vcpu := inst.VCpu
	memory := inst.Memory / 1024
	denom := CpuMemRelation*float64(vcpu) + float64(memory)
	if denom == 0 {
		return 0, 0
	}
	memoryCost = value / denom
	vcpuCost = CpuMemRelation * memoryCost
	return vcpuCost, memoryCost
}
