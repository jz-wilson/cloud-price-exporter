package exporter

import (
	"context"
	"strings"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

func (e *Exporter) getAzureOnDemandPricing(region string, client AzureRetailPricesClient, scrapes chan<- scrapeResult) {
	items, err := client.GetVMPrices(context.TODO(), region, e.azureOperatingSystems)
	if err != nil {
		log.WithError(err).Errorf("error while fetching Azure VM prices [region=%s]", region)
		atomic.AddUint64(&e.errorCount, 1)
		return
	}

	for _, item := range items {
		if len(e.azureInstanceRegexes) > 0 && !isMatchAny(e.azureInstanceRegexes, item.ArmSkuName) {
			log.Debugf("Skipping Azure instance type: %s", item.ArmSkuName)
			continue
		}

		os := classifyAzureOS(item.ProductName)
		if !contains(e.azureOperatingSystems, os) {
			continue
		}

		scrapes <- scrapeResult{
			Name:              "azure_vm",
			Value:             item.RetailPrice,
			Region:            region,
			InstanceType:      item.ArmSkuName,
			InstanceLifecycle: "ondemand",
			OperatingSystem:   os,
		}
	}
}

// classifyAzureOS returns "Windows" if the product name contains "Windows", otherwise "Linux".
func classifyAzureOS(productName string) string {
	if strings.Contains(productName, "Windows") {
		return "Windows"
	}
	return "Linux"
}
