package main

import (
	"context"
	"flag"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/pixelfederation/cloud-price-exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	addr                = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	metricsPath         = flag.String("metrics-path", "/metrics", "path to metrics endpoint")
	rawLevel            = flag.String("log-level", "info", "log level")
	productDescriptions = flag.String("product-descriptions", "Linux/UNIX", "Comma separated list of product descriptions, used to filter spot instances. Accepted values: Linux/UNIX, SUSE Linux, Windows, Linux/UNIX (Amazon VPC), SUSE Linux (Amazon VPC), Windows (Amazon VPC)")
	operatingSystems    = flag.String("operating-systems", "Linux", "Comma separated list of operating systems, used to filter ondemand instances. Accepted values: Linux, RHEL, SUSE, Windows")
	cache           = flag.Int("cache", 0, "How long should the results be cached, in seconds (defaults to *0*)")
	instanceRegexes = flag.String("instance-regexes", "", "Comma separated list of instance types regexes (defaults to *all*)")

	// AWS flags
	awsEnabled      = flag.Bool("aws-enabled", true, "Enable AWS EC2 pricing")
	regions         = flag.String("regions", "", "Comma separated list of AWS regions to get pricing for (defaults to *all*)")
	lifecycle       = flag.String("lifecycle", "", "Comma separated list of Lifecycles (spot or ondemand) to get pricing for (defaults to *all*)")
	savingPlanTypes = flag.String("saving-plan-types", "", "Comma separated list of saving plans types (defaults to *none)")

	// Azure flags
	azureEnabled          = flag.Bool("azure-enabled", true, "Enable Azure VM on-demand pricing")
	azureRegions          = flag.String("azure-regions", "", "Comma separated list of Azure regions (required when azure-enabled)")
	azureOperatingSystems = flag.String("azure-operating-systems", "Linux", "Comma separated list of Azure OS types: Linux, Windows")
	azureInstanceRegexes  = flag.String("azure-instance-regexes", "", "Comma separated list of Azure instance type regexes (defaults to *all*)")
)

func main() {
	flag.Parse()
	parsedLevel, err := log.ParseLevel(*rawLevel)
	if err != nil {
		log.WithError(err).Warnf("Couldn't parse log level, using default: %s", log.GetLevel())
	} else {
		log.SetLevel(parsedLevel)
		log.Debugf("Set log level to %s", parsedLevel)
	}

	log.Infof("Starting Cloud Price exporter. [log-level=%s, aws-enabled=%v, regions=%s, azure-enabled=%v, azure-regions=%s, cache=%d]", *rawLevel, *awsEnabled, *regions, *azureEnabled, *azureRegions, *cache)

	if !*awsEnabled && !*azureEnabled {
		log.Fatal("At least one provider must be enabled (--aws-enabled or --azure-enabled)")
	}

	// --- AWS setup ---
	var reg []string
	var pds, oss, lc, spt []string
	var instRegCompiled []*regexp.Regexp

	if *awsEnabled {
		if len(*regions) == 0 {
			cfg, err := config.LoadDefaultConfig(context.TODO())
			if err != nil {
				log.WithError(err).Errorf("error while initializing aws client to list available regions")
				return
			}

			ec2Svc := ec2.NewFromConfig(cfg)
			r, err := ec2Svc.DescribeRegions(context.TODO(), &ec2.DescribeRegionsInput{AllRegions: aws.Bool(false)})
			if err != nil {
				log.Fatal(err)
				return
			}

			for _, region := range r.Regions {
				reg = append(reg, *region.RegionName)
			}
		} else {
			reg = splitAndTrim(*regions)
		}

		pds = splitAndTrim(*productDescriptions)
		oss = splitAndTrim(*operatingSystems)
		lc = splitAndTrim(*lifecycle)
		if len(lc) == 0 {
			lc = []string{"spot", "ondemand"}
		}
		spt = splitAndTrim(*savingPlanTypes)

		if err := validateProductDesc(pds); err != nil {
			log.Fatal(err)
		}
		if err := validateOperatingSystems(oss); err != nil {
			log.Fatal(err)
		}
		if err := validateSavingPlanTypes(spt); err != nil {
			log.Fatal(err)
		}
	}

	instReg := splitAndTrim(*instanceRegexes)
	if len(instReg) == 0 {
		instReg = []string{".*"}
	}

	if compiled, err := compileRegexes(instReg); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	} else {
		instRegCompiled = compiled
	}

	// --- Azure setup ---
	var azureCfg *exporter.AzureConfig
	if *azureEnabled {
		azureReg := splitAndTrim(*azureRegions)
		if len(azureReg) == 0 {
			log.Warn("Azure enabled but no --azure-regions specified, skipping Azure")
		} else {
			azureOSS := splitAndTrim(*azureOperatingSystems)
			if len(azureOSS) == 0 {
				azureOSS = []string{"Linux"}
			}
			azureInstReg := splitAndTrim(*azureInstanceRegexes)
			if len(azureInstReg) == 0 {
				azureInstReg = []string{".*"}
			}
			azureInstRegCompiled, err := compileRegexes(azureInstReg)
			if err != nil {
				log.Fatalf("invalid azure instance regex: %v", err)
			}
			azureCfg = &exporter.AzureConfig{
				Regions:          azureReg,
				OperatingSystems: azureOSS,
				InstanceRegexes:  azureInstRegCompiled,
				ClientFactory:    exporter.NewDefaultAzureClientFactory(),
			}
		}
	}

	exp, err := exporter.NewExporter(pds, oss, reg, lc, *cache, instRegCompiled, spt, &exporter.AWSClientFactory{}, azureCfg)
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(exp)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", rootHandler)

	srv := &http.Server{
		Addr:         *addr,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigCh
		log.Infof("Received %s, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Infof("Starting metric http endpoint [address=%s, path=%s]", *addr, *metricsPath)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func splitAndTrim(str string) []string {
	if str == "" {
		return []string{}
	}
	parts := strings.Split(str, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func validateProductDesc(pds []string) error {
	for _, desc := range pds {
		if desc != "Linux/UNIX" && desc != "Linux/UNIX (Amazon VPC)" &&
			desc != "SUSE Linux" && desc != "SUSE Linux (Amazon VPC)" &&
			desc != "Windows" && desc != "Windows (Amazon VPC)" {
			return fmt.Errorf("product description '%s' is not recognized. Available product descriptions: Linux/UNIX, SUSE Linux, Windows, Linux/UNIX (Amazon VPC), SUSE Linux (Amazon VPC), Windows (Amazon VPC)", desc)
		}
	}
	return nil
}

func validateOperatingSystems(oss []string) error {
	for _, os := range oss {
		if os != "Linux" &&
			os != "RHEL" &&
			os != "SUSE" &&
			os != "Windows" {
			return fmt.Errorf("operating System '%s' is not recognized. Available operating system: Linux, RHEL, SUSE, Windows", os)
		}
	}
	return nil
}

func validateSavingPlanTypes(spt []string) error {
	for _, plan := range spt {
		if plan != "" &&
			plan != "Compute" &&
			plan != "EC2Instance" &&
			plan != "SageMaker" {
			return fmt.Errorf("savingPlan type '%s' is not recognized. Available SavingPlans types: Compute, EC2Instance, SageMaker", plan)
		}
	}
	return nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	safePath := html.EscapeString(*metricsPath)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<html>
		<head><title>Cloud Price Exporter</title></head>
		<body>
		<h1>Cloud Price Exporter</h1>
		<p><a href="` + safePath + `">Metrics</a></p>
		</body>
		</html>
	`))
}

func compileRegexes(regexes []string) ([]*regexp.Regexp, error) {
	compiledRegexes := make([]*regexp.Regexp, len(regexes))
	for i, r := range regexes {
		re, err := regexp.Compile(r)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %s: %s", r, err)
		}
		compiledRegexes[i] = re
	}
	return compiledRegexes, nil
}
