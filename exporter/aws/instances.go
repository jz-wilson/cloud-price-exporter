package aws

import (
	"context"
	"fmt"
	"strconv"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	log "github.com/sirupsen/logrus"
)

// InstanceStore caches EC2 instance type specifications (vCPU, memory).
type InstanceStore struct {
	instances map[string]Instance
}

// NewInstanceStore returns an empty InstanceStore.
func NewInstanceStore() *InstanceStore {
	return &InstanceStore{instances: make(map[string]Instance)}
}

// NewInstanceStoreFromMap creates an InstanceStore pre-populated with the given instances.
func NewInstanceStoreFromMap(instances map[string]Instance) *InstanceStore {
	return &InstanceStore{instances: instances}
}

// Load fetches all instance types from EC2 and populates the store.
func (s *InstanceStore) Load(ctx context.Context, client ec2.DescribeInstanceTypesAPIClient) error {
	s.instances = make(map[string]Instance)
	pag := ec2.NewDescribeInstanceTypesPaginator(
		client,
		&ec2.DescribeInstanceTypesInput{})
	for pag.HasMorePages() {
		instances, err := pag.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("error fetching available instance types: %w", err)
		}
		for _, instance := range instances.InstanceTypes {
			var memMiB int64
			if instance.MemoryInfo != nil {
				memMiB = awssdk.ToInt64(instance.MemoryInfo.SizeInMiB)
			}
			var vcpus int32
			if instance.VCpuInfo != nil {
				vcpus = awssdk.ToInt32(instance.VCpuInfo.DefaultVCpus)
			}
			s.instances[string(instance.InstanceType)] = Instance{
				Memory: memMiB,
				VCpu:   vcpus,
			}
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
