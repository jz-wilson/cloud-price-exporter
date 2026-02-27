package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"sync/atomic"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	log "github.com/sirupsen/logrus"

	"github.com/pixelfederation/cloud-price-exporter/exporter/provider"
)

// BulkPricingURLFormat is the URL template for the AWS public bulk pricing endpoint.
// %s is replaced with the region code.
var BulkPricingURLFormat = "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/%s/index.json"

// BulkPricingResponse represents the top-level structure of the AWS bulk pricing JSON.
type BulkPricingResponse struct {
	Products map[string]BulkProduct `json:"products"`
	Terms    BulkTerms              `json:"terms"`
}

// BulkProduct represents a single product entry in the bulk pricing JSON.
type BulkProduct struct {
	SKU           string            `json:"sku"`
	ProductFamily string            `json:"productFamily"`
	Attributes    map[string]string `json:"attributes"`
}

// BulkTerms contains on-demand pricing terms from the bulk pricing JSON.
type BulkTerms struct {
	OnDemand map[string]map[string]BulkOfferTerm `json:"OnDemand"`
}

// BulkOfferTerm represents a single offer term in the bulk pricing JSON.
type BulkOfferTerm struct {
	OfferTermCode   string                         `json:"offerTermCode"`
	PriceDimensions map[string]BulkPriceDimension `json:"priceDimensions"`
}

// BulkPriceDimension represents pricing details in the bulk pricing JSON.
type BulkPriceDimension struct {
	PricePerUnit map[string]string `json:"pricePerUnit"`
}

// GetOnDemandPricing fetches on-demand prices from the AWS public bulk pricing
// URL for a region and sends results to scrapes. No AWS credentials are required.
// If ec2Client is nil, the region name is used as the sole availability zone.
// If httpClient is nil, http.DefaultClient is used.
func GetOnDemandPricing(ctx context.Context, region string, ec2Client EC2DescribeAZsAPI, httpClient *http.Client, operatingSystems []string, instanceRegexes []*regexp.Regexp, instances *InstanceStore, errorCount *uint64, scrapes chan<- provider.ScrapeResult) {
	var azs []string
	if ec2Client != nil {
		var err error
		azs, err = GetAZs(ctx, region, ec2Client)
		if err != nil {
			log.WithError(err).Errorf("error while fetching AZs [region=%s]", region)
			atomic.AddUint64(errorCount, 1)
			return
		}
	} else {
		azs = []string{region}
	}

	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	url := fmt.Sprintf(BulkPricingURLFormat, region)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.WithError(err).Errorf("error creating request for bulk pricing [region=%s]", region)
		atomic.AddUint64(errorCount, 1)
		return
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.WithError(err).Errorf("error fetching bulk pricing [region=%s]", region)
		atomic.AddUint64(errorCount, 1)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Errorf("bulk pricing API returned status %d [region=%s]", resp.StatusCode, region)
		atomic.AddUint64(errorCount, 1)
		return
	}

	var bulk BulkPricingResponse
	if err := json.NewDecoder(resp.Body).Decode(&bulk); err != nil {
		log.WithError(err).Errorf("error decoding bulk pricing JSON [region=%s]", region)
		atomic.AddUint64(errorCount, 1)
		return
	}

	osSet := make(map[string]bool, len(operatingSystems))
	for _, os := range operatingSystems {
		osSet[os] = true
	}

	for sku, product := range bulk.Products {
		attrs := product.Attributes

		if attrs["capacitystatus"] != "Used" {
			continue
		}
		if attrs["tenancy"] != "Shared" {
			continue
		}
		if attrs["preInstalledSw"] != "NA" {
			continue
		}
		if !osSet[attrs["operatingSystem"]] {
			continue
		}
		if !provider.IsMatchAny(instanceRegexes, attrs["instanceType"]) {
			log.Debugf("Skipping instance type: %s", attrs["instanceType"])
			continue
		}

		skuOnDemand := fmt.Sprintf("%s.%s", sku, TermOnDemand)
		skuOnDemandPerHour := fmt.Sprintf("%s.%s", skuOnDemand, TermPerHour)

		skuTerms, ok := bulk.Terms.OnDemand[sku]
		if !ok {
			continue
		}
		offerTerm, ok := skuTerms[skuOnDemand]
		if !ok {
			continue
		}
		dim, ok := offerTerm.PriceDimensions[skuOnDemandPerHour]
		if !ok {
			continue
		}
		usdPrice, ok := dim.PricePerUnit["USD"]
		if !ok {
			continue
		}

		value, err := strconv.ParseFloat(usdPrice, 64)
		if err != nil {
			log.WithError(err).Errorf("error while parsing ondemand price value from API response [region=%s, type=%s]", region, attrs["instanceType"])
			atomic.AddUint64(errorCount, 1)
			continue
		}
		log.Debugf("Creating new metric: ec2{region=%s, instance_type=%s, product_description=%s} = %v.", region, attrs["instanceType"], attrs["operatingSystem"], value)

		vcpu, memory := instances.GetNormalizedCost(value, attrs["instanceType"])
		for _, az := range azs {
			scrapes <- provider.ScrapeResult{
				Name:               "ec2",
				Value:              value,
				Region:             region,
				AvailabilityZone:   az,
				InstanceType:       attrs["instanceType"],
				InstanceLifecycle:  "ondemand",
				OperatingSystem:    attrs["operatingSystem"],
				ProductDescription: attrs["productDescription"],
				Memory:             instances.GetMemory(attrs["instanceType"]),
				VCpu:               instances.GetVCpu(attrs["instanceType"]),
			}
			scrapes <- provider.ScrapeResult{
				Name:              "ec2_memory",
				Value:             memory,
				Region:            region,
				AvailabilityZone:  az,
				InstanceType:      attrs["instanceType"],
				InstanceLifecycle: "ondemand",
			}
			scrapes <- provider.ScrapeResult{
				Name:              "ec2_vcpu",
				Value:             vcpu,
				Region:            region,
				AvailabilityZone:  az,
				InstanceType:      attrs["instanceType"],
				InstanceLifecycle: "ondemand",
			}
		}
	}
}

// GetAZs returns the availability zone names for a region.
func GetAZs(ctx context.Context, region string, client EC2DescribeAZsAPI) ([]string, error) {
	tmpazs, err := client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2types.Filter{
			{
				Name:   awssdk.String("group-name"),
				Values: []string{region},
			},
		}})

	if err != nil {
		return nil, fmt.Errorf("couldn't describe AZs in %s: %w", region, err)
	}

	azs := make([]string, len(tmpazs.AvailabilityZones))
	for i, az := range tmpazs.AvailabilityZones {
		azs[i] = *az.ZoneName
	}

	return azs, nil
}
