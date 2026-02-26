package exporter

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestGetInstances_Success(t *testing.T) {
	client := &mockEC2Client{
		DescribeInstanceTypesFn: func(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
			return &ec2.DescribeInstanceTypesOutput{
				InstanceTypes: []ec2types.InstanceTypeInfo{
					{
						InstanceType: ec2types.InstanceTypeM5Large,
						MemoryInfo:   &ec2types.MemoryInfo{SizeInMiB: aws.Int64(8192)},
						VCpuInfo:     &ec2types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
					},
					{
						InstanceType: ec2types.InstanceTypeM5Xlarge,
						MemoryInfo:   &ec2types.MemoryInfo{SizeInMiB: aws.Int64(16384)},
						VCpuInfo:     &ec2types.VCpuInfo{DefaultVCpus: aws.Int32(4)},
					},
				},
			}, nil
		},
	}

	e := &Exporter{}
	err := e.getInstances(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(e.instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(e.instances))
	}

	m5large := e.instances["m5.large"]
	if m5large.Memory != 8192 || m5large.VCpu != 2 {
		t.Errorf("m5.large: expected Memory=8192 VCpu=2, got Memory=%d VCpu=%d", m5large.Memory, m5large.VCpu)
	}

	m5xl := e.instances["m5.xlarge"]
	if m5xl.Memory != 16384 || m5xl.VCpu != 4 {
		t.Errorf("m5.xlarge: expected Memory=16384 VCpu=4, got Memory=%d VCpu=%d", m5xl.Memory, m5xl.VCpu)
	}
}

func TestGetInstances_APIError(t *testing.T) {
	client := &mockEC2Client{
		DescribeInstanceTypesFn: func(ctx context.Context, params *ec2.DescribeInstanceTypesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	e := &Exporter{}
	err := e.getInstances(client)
	if err == nil {
		t.Fatal("expected error from getInstances, got nil")
	}
}

func TestGetNormalizedCost(t *testing.T) {
	e := &Exporter{
		instances: map[string]Instance{
			"m5.large": {Memory: 8192, VCpu: 2},
		},
	}

	// m5.large: 2 vCPUs, 8192 MiB = 8 GiB
	// memoryCost = 0.096 / (7.2*2 + 8) = 0.096 / 22.4
	// vcpuCost = 7.2 * memoryCost
	vcpu, memory := e.getNormalizedCost(0.096, "m5.large")

	expectedMemory := 0.096 / (7.2*2 + 8)
	expectedVCpu := 7.2 * expectedMemory

	if math.Abs(vcpu-expectedVCpu) > 1e-10 {
		t.Errorf("vcpu cost: expected %v, got %v", expectedVCpu, vcpu)
	}
	if math.Abs(memory-expectedMemory) > 1e-10 {
		t.Errorf("memory cost: expected %v, got %v", expectedMemory, memory)
	}
}

func TestGetNormalizedCost_UnknownInstance(t *testing.T) {
	e := &Exporter{
		instances: map[string]Instance{},
	}

	// Unknown instance — should return 0, 0 instead of divide-by-zero.
	vcpu, memory := e.getNormalizedCost(0.1, "unknown.type")

	if vcpu != 0 {
		t.Errorf("expected 0 for unknown instance vcpu, got %v", vcpu)
	}
	if memory != 0 {
		t.Errorf("expected 0 for unknown instance memory, got %v", memory)
	}
}

func TestGetInstanceMemory(t *testing.T) {
	e := &Exporter{
		instances: map[string]Instance{
			"m5.large": {Memory: 8192, VCpu: 2},
		},
	}

	if got := e.getInstanceMemory("m5.large"); got != "8192" {
		t.Errorf("expected 8192, got %s", got)
	}

	// Unknown instance returns "0"
	if got := e.getInstanceMemory("unknown"); got != "0" {
		t.Errorf("expected 0 for unknown instance, got %s", got)
	}
}

func TestGetInstanceVCpu(t *testing.T) {
	e := &Exporter{
		instances: map[string]Instance{
			"m5.large": {Memory: 8192, VCpu: 2},
		},
	}

	if got := e.getInstanceVCpu("m5.large"); got != "2" {
		t.Errorf("expected 2, got %s", got)
	}

	if got := e.getInstanceVCpu("unknown"); got != "0" {
		t.Errorf("expected 0 for unknown instance, got %s", got)
	}
}
