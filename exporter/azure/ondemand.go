package azure

import (
	"context"
	"regexp"
	"strings"
	"sync/atomic"

	log "github.com/sirupsen/logrus"

	"github.com/pixelfederation/cloud-price-exporter/exporter/provider"
)

// GetOnDemandPricing fetches Azure VM on-demand prices for a single region
// and sends results to the scrapes channel.
func GetOnDemandPricing(ctx context.Context, region string, client RetailPricesClient, operatingSystems []string, instanceRegexes []*regexp.Regexp, errorCount *uint64, scrapes chan<- provider.ScrapeResult) {
	items, err := client.GetVMPrices(ctx, region, operatingSystems)
	if err != nil {
		log.WithError(err).Errorf("error while fetching Azure VM prices [region=%s]", region)
		atomic.AddUint64(errorCount, 1)
		return
	}

	for _, item := range items {
		if len(instanceRegexes) > 0 && !provider.IsMatchAny(instanceRegexes, item.ArmSkuName) {
			log.Debugf("Skipping Azure instance type: %s", item.ArmSkuName)
			continue
		}

		os := classifyAzureOS(item.ProductName)
		if !provider.Contains(operatingSystems, os) {
			continue
		}

		scrapes <- provider.ScrapeResult{
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
