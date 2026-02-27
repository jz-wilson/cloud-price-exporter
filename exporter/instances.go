package exporter

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	log "github.com/sirupsen/logrus"
)

const (
	// AWS doesn't share the relationship between CPU and memory for each instance type, therefore we get this info from GCP.
	// Obviously, it could be some differences between the cpu/memory relationship between the cloud providers but using the GCP
	// relationship could give us a fairly approximate global idea and allow us know the cost of our pods and namespaces.

	// To simplify operations and taking into account an approximate global idea would be accepted the CPU-Memory relationship is
	// calculated as:

	// CPU-cost = 7.2 memory-GB-cost

	// https://engineering.empathy.co/cloud-finops-part-4-kubernetes-cost-report/
	cpuMemRelation = 7.2
)

func (e *Exporter) getInstances(client ec2.DescribeInstanceTypesAPIClient) error {
	e.instances = make(map[string]Instance)
	pag := ec2.NewDescribeInstanceTypesPaginator(
		client,
		&ec2.DescribeInstanceTypesInput{})
	for pag.HasMorePages() {
		instances, err := pag.NextPage(context.TODO())
		if err != nil {
			return fmt.Errorf("error fetching available instance types: %w", err)
		}
		for _, instance := range instances.InstanceTypes {
			e.instances[string(instance.InstanceType)] = Instance{
				Memory: aws.ToInt64(instance.MemoryInfo.SizeInMiB),
				VCpu:   aws.ToInt32(instance.VCpuInfo.DefaultVCpus),
			}
		}
	}

	log.Infof("loaded %d instance types", len(e.instances))
	return nil
}

func (e *Exporter) getInstanceMemory(instance string) string {
	return strconv.Itoa(int(e.instances[instance].Memory))
}

func (e *Exporter) getInstanceVCpu(instance string) string {
	return strconv.Itoa(int(e.instances[instance].VCpu))
}

func (e *Exporter) getNormalizedCost(value float64, instance string) (float64, float64) {
	inst, ok := e.instances[instance]
	if !ok {
		return 0, 0
	}
	vcpu := inst.VCpu
	memory := inst.Memory / 1024
	denom := cpuMemRelation*float64(vcpu) + float64(memory)
	if denom == 0 {
		return 0, 0
	}
	memoryCost := value / denom
	vcpuCost := cpuMemRelation * memoryCost
	return vcpuCost, memoryCost
}
