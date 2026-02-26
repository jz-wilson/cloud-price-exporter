package exporter

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const (
	AwsMaxResultsPerPage int32 = 100
)

// AzureConfig holds configuration for Azure VM pricing scraping.
// Pass nil to NewExporter to disable Azure.
type AzureConfig struct {
	Regions          []string
	OperatingSystems []string
	InstanceRegexes  []*regexp.Regexp
	ClientFactory    AzureClientFactory
}

// Exporter implements the prometheus.Exporter interface, and exports AWS EC2 Price metrics.
type Exporter struct {
	productDescriptions []string
	operatingSystems    []string
	regions             []string
	lifecycle           []string
	duration            prometheus.Gauge
	scrapeErrors        prometheus.Gauge
	totalScrapes        prometheus.Counter
	pricingMetrics      map[string]*prometheus.GaugeVec
	instances           map[string]Instance
	instanceRegexes     []*regexp.Regexp
	savingPlanTypes     []string
	clientFactory       ClientFactory
	cache               int
	nextScrape          time.Time
	errorCount          uint64
	metricsMtx sync.RWMutex
	mu         sync.RWMutex

	// Azure fields
	azureEnabled          bool
	azureRegions          []string
	azureOperatingSystems []string
	azureInstanceRegexes  []*regexp.Regexp
	azureClientFactory    AzureClientFactory
}

type scrapeResult struct {
	Name               string
	Value              float64
	Region             string
	AvailabilityZone   string
	InstanceType       string
	InstanceLifecycle  string
	ProductDescription string
	OperatingSystem    string
	SavingPlanOption   string
	SavingPlanDuration int
	SavingPlanType     string
	Memory             string
	VCpu               string
}

// NewExporter returns a new exporter of AWS EC2 Price metrics.
// Pass nil for azureCfg to disable Azure VM pricing.
func NewExporter(pds []string, oss []string, regions []string, lifecycle []string, cache int, instanceRegexes []*regexp.Regexp, savingPlanTypes []string, clientFactory ClientFactory, azureCfg *AzureConfig) (*Exporter, error) {

	e := Exporter{
		productDescriptions: pds,
		operatingSystems:    oss,
		regions:             regions,
		lifecycle:           lifecycle,
		cache:               cache,
		instanceRegexes:     instanceRegexes,
		savingPlanTypes:     savingPlanTypes,
		clientFactory:       clientFactory,
		nextScrape:          time.Now(),
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "aws_pricing",
			Name:      "scrape_duration_seconds",
			Help:      "The scrape duration.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "aws_pricing",
			Name:      "scrapes_total",
			Help:      "Total AWS autoscaling group scrapes.",
		}),
		scrapeErrors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "aws_pricing",
			Name:      "scrape_error",
			Help:      "The scrape error status.",
		}),
	}

	if azureCfg != nil {
		e.azureEnabled = true
		e.azureRegions = azureCfg.Regions
		e.azureOperatingSystems = azureCfg.OperatingSystems
		e.azureInstanceRegexes = azureCfg.InstanceRegexes
		e.azureClientFactory = azureCfg.ClientFactory
	}

	e.initGauges()

	// Only fetch AWS instances if AWS regions are configured
	if len(regions) > 0 {
		ec2Client, err := e.clientFactory.NewEC2Client("us-east-1")
		if err != nil {
			return nil, fmt.Errorf("failed to create EC2 client: %w", err)
		}
		if err := e.getInstances(ec2Client); err != nil {
			return nil, err
		}
	}

	return &e, nil
}

func (e *Exporter) initGauges() {
	e.pricingMetrics = map[string]*prometheus.GaugeVec{}
	e.pricingMetrics["ec2"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "aws_pricing",
		Name:      "ec2",
		Help:      "Current price of the instance type.",
	}, []string{"instance_lifecycle", "instance_type", "region", "availability_zone", "product_description", "operating_system", "saving_plan_option", "saving_plan_duration", "saving_plan_type", "memory", "vcpu"})

	e.pricingMetrics["ec2_memory"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "aws_pricing",
		Name:      "ec2_memory",
		Help:      "Price of each GB of memory of the instance.",
	}, []string{"instance_lifecycle", "instance_type", "region", "availability_zone", "saving_plan_option", "saving_plan_duration", "saving_plan_type"})

	e.pricingMetrics["ec2_vcpu"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "aws_pricing",
		Name:      "ec2_vcpu",
		Help:      "Price of each VCPU of the instance.",
	}, []string{"instance_lifecycle", "instance_type", "region", "availability_zone", "saving_plan_option", "saving_plan_duration", "saving_plan_type"})

	if e.azureEnabled {
		e.pricingMetrics["azure_vm"] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "azure_pricing",
			Name:      "vm",
			Help:      "Current price of the Azure VM instance type.",
		}, []string{"instance_lifecycle", "instance_type", "region", "operating_system"})
	}
}

// resetGauges clears all existing gauge values without replacing the registered GaugeVec objects.
func (e *Exporter) resetGauges() {
	for _, m := range e.pricingMetrics {
		m.Reset()
	}
}

// Describe outputs metric descriptions.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range e.pricingMetrics {
		m.Describe(ch)
	}
	ch <- e.duration.Desc()
	ch <- e.totalScrapes.Desc()
	ch <- e.scrapeErrors.Desc()
}

// Collect fetches info from the AWS API
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	now := time.Now()

	if now.After(e.nextScrape) {
		pricingScrapes := make(chan scrapeResult)

		e.mu.Lock()
		defer e.mu.Unlock()

		// Timeout bounds individual API calls within scrape goroutines, not overall scrape duration.
		// setPricingMetrics blocks until the channel drains, so cancel fires after scrapes complete.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		e.resetGauges()
		go e.scrape(ctx, pricingScrapes)
		e.setPricingMetrics(pricingScrapes)

		e.nextScrape = time.Now().Add(time.Second * time.Duration(e.cache))
	}

	e.duration.Collect(ch)
	e.totalScrapes.Collect(ch)
	e.scrapeErrors.Collect(ch)

	for _, m := range e.pricingMetrics {
		m.Collect(ch)
	}
}

func (e *Exporter) scrape(ctx context.Context, scrapes chan<- scrapeResult) {

	defer close(scrapes)
	now := time.Now()

	e.totalScrapes.Inc()

	atomic.StoreUint64(&e.errorCount, 0)
	log.Debugf("before for %v\n", e.regions)

	var wg sync.WaitGroup
	for _, region := range e.regions {
		log.Debugf("querying ec2 prices [region=%s]", region)
		wg.Add(1)
		go func(region string) {
			defer wg.Done()

			ec2Client, err := e.clientFactory.NewEC2Client(region)
			if err != nil {
				log.WithError(err).Errorf("failed to create EC2 client [region=%s]", region)
				atomic.AddUint64(&e.errorCount, 1)
				return
			}

			if contains(e.lifecycle, "spot") {
				e.getSpotPricing(ctx, region, ec2Client, scrapes)
			}

			if contains(e.lifecycle, "ondemand") {
				pricingClient, err := e.clientFactory.NewPricingClient()
				if err != nil {
					log.WithError(err).Errorf("failed to create Pricing client [region=%s]", region)
					atomic.AddUint64(&e.errorCount, 1)
					return
				}
				e.getOnDemandPricing(ctx, region, ec2Client, pricingClient, scrapes)
			}

			if len(e.savingPlanTypes) != 0 {
				spClient, err := e.clientFactory.NewSavingsPlansClient()
				if err != nil {
					log.WithError(err).Errorf("failed to create SavingsPlans client [region=%s]", region)
					atomic.AddUint64(&e.errorCount, 1)
					return
				}
				e.getSavingPlanPricing(ctx, region, spClient, scrapes)
			}

		}(region)
	}
	// Azure VM pricing
	if e.azureEnabled && e.azureClientFactory != nil {
		for _, region := range e.azureRegions {
			wg.Add(1)
			go func(region string) {
				defer wg.Done()
				client := e.azureClientFactory.NewAzureRetailPricesClient()
				e.getAzureOnDemandPricing(ctx, region, client, scrapes)
			}(region)
		}
	}

	wg.Wait()

	e.scrapeErrors.Set(float64(atomic.LoadUint64(&e.errorCount)))
	e.duration.Set(time.Since(now).Seconds())
}

func (e *Exporter) setPricingMetrics(scrapes <-chan scrapeResult) {
	log.Debug("set pricing metrics")
	for scr := range scrapes {
		name := scr.Name
		if _, ok := e.pricingMetrics[name]; !ok {
			e.metricsMtx.Lock()
			if name == "ec2" {
				e.pricingMetrics[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: "aws_pricing",
					Name:      name,
				}, []string{"instance_lifecycle", "instance_type", "region", "availability_zone", "product_description", "operating_system", "memory", "vcpu"})
			} else if name == "ec2_memory" || name == "ec2_vcpu" {
				e.pricingMetrics[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: "aws_pricing",
					Name:      name,
				}, []string{"instance_lifecycle", "instance_type", "region", "availability_zone"})
			} else if name == "azure_vm" {
				e.pricingMetrics[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: "azure_pricing",
					Name:      "vm",
				}, []string{"instance_lifecycle", "instance_type", "region", "operating_system"})
			}
			e.metricsMtx.Unlock()
		}
		var labels prometheus.Labels
		if name == "ec2" {
			labels = map[string]string{
				"instance_lifecycle":   scr.InstanceLifecycle,
				"instance_type":        scr.InstanceType,
				"region":               scr.Region,
				"availability_zone":    scr.AvailabilityZone,
				"product_description":  scr.ProductDescription,
				"operating_system":     scr.OperatingSystem,
				"saving_plan_option":   scr.SavingPlanOption,
				"saving_plan_duration": strconv.Itoa(scr.SavingPlanDuration),
				"saving_plan_type":     scr.SavingPlanType,
				"memory":               scr.Memory,
				"vcpu":                 scr.VCpu,
			}
		} else if name == "ec2_memory" || name == "ec2_vcpu" {
			labels = map[string]string{
				"instance_lifecycle":   scr.InstanceLifecycle,
				"instance_type":        scr.InstanceType,
				"region":               scr.Region,
				"availability_zone":    scr.AvailabilityZone,
				"saving_plan_option":   scr.SavingPlanOption,
				"saving_plan_duration": strconv.Itoa(scr.SavingPlanDuration),
				"saving_plan_type":     scr.SavingPlanType,
			}
		} else if name == "azure_vm" {
			labels = map[string]string{
				"instance_lifecycle": scr.InstanceLifecycle,
				"instance_type":     scr.InstanceType,
				"region":            scr.Region,
				"operating_system":  scr.OperatingSystem,
			}
		}
		e.pricingMetrics[name].With(labels).Set(float64(scr.Value))
	}
}

func contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

func isMatchAny(regexList []*regexp.Regexp, text string) bool {
	for _, regex := range regexList {
		if regex.MatchString(text) {
			return true
		}
	}
	return false
}
