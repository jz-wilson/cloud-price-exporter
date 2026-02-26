package aws

import (
	"context"
	"fmt"
	"math"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestInstanceStore_Load_Success(t *testing.T) {
	client := &mockEC2Client{
		DescribeInstanceTypesFn: func(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
			return &ec2.DescribeInstanceTypesOutput{
				InstanceTypes: []ec2types.InstanceTypeInfo{
					{
						InstanceType: ec2types.InstanceTypeM5Large,
						MemoryInfo:   &ec2types.MemoryInfo{SizeInMiB: awssdk.Int64(8192)},
						VCpuInfo:     &ec2types.VCpuInfo{DefaultVCpus: awssdk.Int32(2)},
					},
					{
						InstanceType: ec2types.InstanceTypeM5Xlarge,
						MemoryInfo:   &ec2types.MemoryInfo{SizeInMiB: awssdk.Int64(16384)},
						VCpuInfo:     &ec2types.VCpuInfo{DefaultVCpus: awssdk.Int32(4)},
					},
				},
			}, nil
		},
	}

	store := NewInstanceStore()
	err := store.Load(context.Background(), client)
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
}

func TestInstanceStore_Load_APIError(t *testing.T) {
	client := &mockEC2Client{
		DescribeInstanceTypesFn: func(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	store := NewInstanceStore()
	err := store.Load(context.Background(), client)
	if err == nil {
		t.Fatal("expected error from Load, got nil")
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
