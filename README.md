# Cloud Price Exporter

Prometheus exporter for cloud VM pricing. Scrapes **AWS EC2** (spot, on-demand, savings plans) and **Azure VM** on-demand pricing, exposing them as Prometheus gauge metrics.

## Metrics

### AWS Metrics

| Metric | Description | Labels |
|--------|-------------|--------|
| `aws_pricing_ec2` | Hourly price of the EC2 instance type | `instance_lifecycle`, `instance_type`, `region`, `availability_zone`, `product_description`, `operating_system`, `saving_plan_option`, `saving_plan_duration`, `saving_plan_type`, `memory`, `vcpu` |
| `aws_pricing_ec2_memory` | Normalized price per GB of memory | `instance_lifecycle`, `instance_type`, `region`, `availability_zone`, `saving_plan_option`, `saving_plan_duration`, `saving_plan_type` |
| `aws_pricing_ec2_vcpu` | Normalized price per vCPU | `instance_lifecycle`, `instance_type`, `region`, `availability_zone`, `saving_plan_option`, `saving_plan_duration`, `saving_plan_type` |

### Azure Metrics

| Metric | Description | Labels |
|--------|-------------|--------|
| `azure_pricing_vm` | Hourly on-demand price of the Azure VM type | `instance_lifecycle`, `instance_type`, `region`, `operating_system` |

Azure pricing is fetched from the [Azure Retail Prices REST API](https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices), which is public and requires **no authentication**.

### Internal Metrics

| Metric | Description |
|--------|-------------|
| `aws_pricing_scrape_duration_seconds` | Time taken for the last scrape (covers both AWS and Azure) |
| `aws_pricing_scrapes_total` | Total number of scrapes performed |
| `aws_pricing_scrape_error` | Error status of the last scrape (0 = success) |

## Quick Start

### Local (Go)

```bash
# Azure only (no AWS credentials needed)
go run . -aws-enabled=false -azure-regions eastus

# AWS only
go run . -azure-enabled=false -regions us-east-1 -lifecycle spot,ondemand

# Both AWS and Azure (both enabled by default)
go run . -regions us-east-1 -lifecycle spot -azure-regions eastus,westus2

# Then scrape metrics
curl http://localhost:8080/metrics
```

### Docker

```bash
docker run -p 8080:8080 ghcr.io/jz-wilson/cloud-price-exporter:1.2.0 \
  -azure-regions eastus
```

### Helm

```bash
helm install cloud-price-exporter ./stable/cloud-price-exporter \
  --set exporter.azure.regions=eastus
```

## CLI Flags

### General

| Flag | Default | Description |
|------|---------|-------------|
| `-listen-address` | `:8080` | Address to listen on for HTTP requests |
| `-metrics-path` | `/metrics` | Path to the metrics endpoint |
| `-log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `-cache` | `0` | Cache duration in seconds (0 = no caching) |
| `-instance-regexes` | `.*` | Comma-separated regexes to filter AWS instance types |

### AWS Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-aws-enabled` | `true` | Enable AWS EC2 pricing |
| `-regions` | *(all)* | Comma-separated AWS regions. Empty auto-discovers all regions (requires AWS credentials) |
| `-lifecycle` | `spot,ondemand` | Comma-separated lifecycle types: `spot`, `ondemand` |
| `-product-descriptions` | `Linux/UNIX` | Spot instance filter. Values: `Linux/UNIX`, `SUSE Linux`, `Windows`, and their `(Amazon VPC)` variants |
| `-operating-systems` | `Linux` | On-demand filter. Values: `Linux`, `RHEL`, `SUSE`, `Windows` |
| `-saving-plan-types` | *(none)* | Comma-separated savings plan types: `Compute`, `EC2Instance`, `SageMaker` |

AWS requires IAM credentials with permissions for `ec2:DescribeSpotPriceHistory`, `ec2:DescribeInstanceTypes`, `ec2:DescribeAvailabilityZones`, `ec2:DescribeRegions`, `pricing:GetProducts`, and `savingsplans:DescribeSavingsPlansOfferingRates`.

### Azure Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-azure-enabled` | `true` | Enable Azure VM pricing |
| `-azure-regions` | *(empty)* | Comma-separated Azure regions (e.g., `eastus`, `westeurope`). Empty = skipped with warning |
| `-azure-operating-systems` | `Linux` | Comma-separated OS types: `Linux`, `Windows` |
| `-azure-instance-regexes` | `.*` | Comma-separated regexes to filter Azure VM SKU names |

Azure requires **no credentials** -- the Retail Prices API is public.

## Operating Modes

Both providers are enabled by default. Disable either with `--aws-enabled=false` or `--azure-enabled=false`.

- **Both** (default): AWS auto-discovers regions; Azure requires `--azure-regions`. Both scrape concurrently.
- **AWS only**: Set `--azure-enabled=false` or omit `--azure-regions` (logs a warning and skips Azure).
- **Azure only**: Set `--aws-enabled=false --azure-regions eastus`. No AWS credentials needed.

## Helm Chart Configuration

The chart is in `stable/cloud-price-exporter/`. All exporter flags map to `values.yaml`:

```yaml
exporter:
  cache: 300
  instanceRegexes: ""
  logLevel: "info"

  aws:
    enabled: true
    regions: ""                    # Empty = auto-discover all
    lifecycle: "spot,ondemand"
    productDescriptions: "Linux/UNIX"
    operatingSystems: "Linux"
    savingPlanTypes: ""

  azure:
    enabled: true
    regions: ""                    # Required when enabled
    operatingSystems: "Linux"
    instanceRegexes: ""
```

### Examples

AWS spot pricing in us-east-1 only:

```yaml
exporter:
  aws:
    regions: "us-east-1"
    lifecycle: "spot"
  azure:
    enabled: false
```

Azure only, two regions, Linux and Windows:

```yaml
exporter:
  aws:
    enabled: false
  azure:
    regions: "eastus,westeurope"
    operatingSystems: "Linux,Windows"
```

Both clouds, filtered instance types:

```yaml
exporter:
  instanceRegexes: "^m5\\.,^c5\\."
  aws:
    regions: "us-east-1"
    lifecycle: "spot,ondemand"
  azure:
    regions: "eastus"
    instanceRegexes: "^Standard_D,^Standard_E"
```

### AWS Authentication on EKS

Use [IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) (IAM Roles for Service Accounts):

```yaml
serviceAccount:
  create: true
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/cloud-price-exporter
```

### ServiceMonitor

Enable for automatic Prometheus Operator discovery:

```yaml
serviceMonitor:
  enabled: true
  interval: "5m"
  scrapeTimeout: "30s"
```

## Architecture

```text
main.go                          CLI flags, config parsing, HTTP server
exporter/
  exporter.go                    Exporter struct, Prometheus Collector interface, scrape orchestration
  clients.go                     AWS client interfaces (EC2Client, ClientFactory)
  aws_clients.go                 Production AWS SDK client factory
  spot.go                        AWS spot pricing scraper
  ondemand.go                    AWS on-demand pricing scraper
  savingplan.go                  AWS savings plan pricing scraper
  instances.go                   Instance type metadata (memory, vCPU) + normalized cost calculation
  types.go                       AWS API response types
  azure_types.go                 Azure Retail Prices API response types
  azure_clients.go               Azure client interfaces (AzureRetailPricesClient, AzureClientFactory)
  azure_retail_client.go         Production HTTP client for Azure Retail Prices API
  azure_ondemand.go              Azure VM on-demand pricing scraper
```

### How Scraping Works

1. Prometheus calls `Collect()` on the exporter
2. If the cache has expired, a new scrape begins
3. Each AWS region and Azure region spawns a concurrent goroutine
4. Each goroutine creates its own API clients via the factory pattern (avoids data races)
5. Results stream through a `scrapeResult` channel to `setPricingMetrics()`
6. Metrics are set on the appropriate `prometheus.GaugeVec` by name (`ec2`, `ec2_memory`, `ec2_vcpu`, `azure_vm`)

### Azure API Details

The Azure scraper queries the [Retail Prices API](https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices) with this OData filter:

```text
serviceName eq 'Virtual Machines'
  and priceType eq 'Consumption'
  and armRegionName eq '{region}'
  and isPrimaryMeterRegion eq true
```

Client-side filtering then excludes:
- Non-hourly items (`UnitOfMeasure != "1 Hour"`)
- Spot instances (`MeterName` contains "Spot")
- Low priority instances (`MeterName` contains "Low Priority")

OS classification is derived from `ProductName` -- if it contains "Windows", it's classified as Windows; otherwise Linux.

## Development

### Run Tests

```bash
# Unit tests (no credentials needed)
go test -race ./...

# Integration tests (Azure: no auth; AWS: needs credentials)
go test -tags integration -v ./exporter/

# Coverage
go test -cover ./...
```

### Build

```bash
go build -o cloud-price-exporter .
```

### Lint Helm Chart

```bash
helm lint stable/cloud-price-exporter
helm template test stable/cloud-price-exporter --set exporter.azure.regions=eastus
```
