package exporter

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/jz-wilson/cloud-price-exporter/exporter/aws"
	"github.com/jz-wilson/cloud-price-exporter/exporter/azure"
	"github.com/jz-wilson/cloud-price-exporter/exporter/provider"
)

// AzureConfig holds configuration for Azure VM pricing scraping.
// Pass nil to NewExporter to disable Azure.
type AzureConfig struct {
	Regions          []string
	OperatingSystems []string
	InstanceRegexes  []*regexp.Regexp
	ClientFactory    azure.ClientFactory
}

// Exporter implements the prometheus.Collector interface and exports cloud pricing metrics.
type Exporter struct {
	// AWS fields
	productDescriptions []string
	operatingSystems    []string
	regions             []string
	lifecycle           []string
	instanceRegexes     []*regexp.Regexp
	savingPlanTypes     []string
	clientFactory       aws.ClientFactory
	instances           *aws.InstanceStore
	cache               int

	// Azure fields
	azureEnabled          bool
	azureRegions          []string
	azureOperatingSystems []string
	azureInstanceRegexes  []*regexp.Regexp
	azureClientFactory    azure.ClientFactory

	// Prometheus metrics
	duration       prometheus.Gauge
	scrapeErrors   prometheus.Gauge
	totalScrapes   prometheus.Counter
	pricingMetrics map[string]*prometheus.GaugeVec

	// State
	nextScrape time.Time
	errorCount uint64
	mu         sync.Mutex
}

// NewExporter returns a new exporter of cloud pricing metrics.
// Pass nil for azureCfg to disable Azure VM pricing.
func NewExporter(pds []string, oss []string, regions []string, lifecycle []string, cache int, instanceRegexes []*regexp.Regexp, savingPlanTypes []string, clientFactory aws.ClientFactory, azureCfg *AzureConfig) (*Exporter, error) {

	e := Exporter{
		productDescriptions: pds,
		operatingSystems:    oss,
		regions:             regions,
		lifecycle:           lifecycle,
		cache:               cache,
		instanceRegexes:     instanceRegexes,
		savingPlanTypes:     savingPlanTypes,
		clientFactory:       clientFactory,
		instances:           aws.NewInstanceStore(),
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
		if err := e.instances.Load(context.Background(), nil); err != nil {
			log.WithError(err).Warn("failed to load instance metadata from ec2instances.info — normalized vCPU/memory costs will be unavailable; pricing metrics will still be collected")
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

// Collect fetches info from cloud provider APIs.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {

	e.mu.Lock()
	if time.Now().After(e.nextScrape) {
		// Set nextScrape immediately to prevent concurrent scrapes from entering
		e.nextScrape = time.Now().Add(time.Second * time.Duration(e.cache))

		pricingScrapes := make(chan provider.ScrapeResult)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		e.resetGauges()
		go e.scrape(ctx, pricingScrapes)
		e.setPricingMetrics(pricingScrapes)
	}
	e.mu.Unlock()

	e.duration.Collect(ch)
	e.totalScrapes.Collect(ch)
	e.scrapeErrors.Collect(ch)

	for _, m := range e.pricingMetrics {
		m.Collect(ch)
	}
}

func (e *Exporter) scrape(ctx context.Context, scrapes chan<- provider.ScrapeResult) {

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

			if provider.Contains(e.lifecycle, "spot") {
				aws.GetSpotPricing(ctx, region, ec2Client, e.productDescriptions, e.instanceRegexes, e.instances, &e.errorCount, scrapes)
			}

			if provider.Contains(e.lifecycle, "ondemand") {
				aws.GetOnDemandPricing(ctx, region, ec2Client, nil, e.operatingSystems, e.instanceRegexes, e.instances, &e.errorCount, scrapes)
			}

			if len(e.savingPlanTypes) != 0 {
				spClient, err := e.clientFactory.NewSavingsPlansClient()
				if err != nil {
					log.WithError(err).Errorf("failed to create SavingsPlans client [region=%s]", region)
					atomic.AddUint64(&e.errorCount, 1)
					return
				}
				aws.GetSavingPlanPricing(ctx, region, spClient, e.savingPlanTypes, e.productDescriptions, e.instanceRegexes, e.instances, &e.errorCount, scrapes)
			}

		}(region)
	}
	// Azure VM pricing
	if e.azureEnabled && e.azureClientFactory != nil {
		for _, region := range e.azureRegions {
			wg.Add(1)
			go func(region string) {
				defer wg.Done()
				client := e.azureClientFactory.NewRetailPricesClient()
				azure.GetOnDemandPricing(ctx, region, client, e.azureOperatingSystems, e.azureInstanceRegexes, &e.errorCount, scrapes)
			}(region)
		}
	}

	wg.Wait()

	e.scrapeErrors.Set(float64(atomic.LoadUint64(&e.errorCount)))
	e.duration.Set(time.Since(now).Seconds())
}

func (e *Exporter) setPricingMetrics(scrapes <-chan provider.ScrapeResult) {
	log.Debug("set pricing metrics")
	for scr := range scrapes {
		name := scr.Name
		if _, ok := e.pricingMetrics[name]; !ok {
			log.Warnf("setPricingMetrics: unknown metric name %q, dropping", name)
			continue
		}
		var labels prometheus.Labels
		switch name {
		case "ec2":
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
		case "ec2_memory", "ec2_vcpu":
			labels = map[string]string{
				"instance_lifecycle":   scr.InstanceLifecycle,
				"instance_type":        scr.InstanceType,
				"region":               scr.Region,
				"availability_zone":    scr.AvailabilityZone,
				"saving_plan_option":   scr.SavingPlanOption,
				"saving_plan_duration": strconv.Itoa(scr.SavingPlanDuration),
				"saving_plan_type":     scr.SavingPlanType,
			}
		case "azure_vm":
			labels = map[string]string{
				"instance_lifecycle": scr.InstanceLifecycle,
				"instance_type":      scr.InstanceType,
				"region":             scr.Region,
				"operating_system":   scr.OperatingSystem,
			}
		}
		e.pricingMetrics[name].With(labels).Set(float64(scr.Value))
	}
}
