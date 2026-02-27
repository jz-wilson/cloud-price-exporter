# Cloud Price Exporter

[![CI](https://github.com/jz-wilson/cloud-price-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/jz-wilson/cloud-price-exporter/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jz-wilson/cloud-price-exporter)](https://github.com/jz-wilson/cloud-price-exporter/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jz-wilson/cloud-price-exporter)](https://go.dev/doc/install)
[![Go Report Card](https://goreportcard.com/badge/github.com/jz-wilson/cloud-price-exporter)](https://goreportcard.com/report/github.com/jz-wilson/cloud-price-exporter)
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue?logo=docker)](https://github.com/jz-wilson/cloud-price-exporter/pkgs/container/cloud-price-exporter)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

Prometheus exporter for cloud VM pricing. Scrapes **AWS EC2** (spot, on-demand, savings plans) and **Azure VM** on-demand pricing, exposing them as Prometheus gauge metrics.

## Credentials Required

| Feature | Credentials |
|---|---|
| AWS on-demand pricing | ✅ None — fetched from [AWS public bulk pricing](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/) |
| AWS instance metadata (vCPU/memory) | ✅ None — fetched from [ec2instances.info](https://ec2instances.info) |
| Azure VM pricing | ✅ None — fetched from [Azure Retail Prices REST API](https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices) |
| AWS spot pricing | ⚠️ IAM credentials required (`ec2:DescribeSpotPriceHistory`, `ec2:DescribeAvailabilityZones`) |
| AWS savings plans | ⚠️ IAM credentials required (`savingsplans:DescribeSavingsPlansOfferingRates`) |

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

### Internal Metrics

| Metric | Description |
|--------|-------------|
| `aws_pricing_scrape_duration_seconds` | Time taken for the last scrape |
| `aws_pricing_scrapes_total` | Total number of scrapes performed |
| `aws_pricing_scrape_error` | Error status of the last scrape (0 = success) |

## Quick Start

### Local (Go)

```bash
# Azure only — no credentials needed
go run . -aws-enabled=false -azure-regions eastus

# AWS on-demand only — no credentials needed
go run . -azure-enabled=false -regions us-east-1 -lifecycle ondemand

# AWS spot (requires IAM credentials)
go run . -azure-enabled=false -regions us-east-1 -lifecycle spot

# Both AWS and Azure
go run . -regions us-east-1 -lifecycle spot,ondemand -azure-regions eastus,westus2

# Scrape metrics
curl http://localhost:8080/metrics
```

### Docker

```bash
# Azure only — no credentials needed
docker run -p 8080:8080 ghcr.io/jz-wilson/cloud-price-exporter:latest \
  -aws-enabled=false -azure-regions eastus

# AWS on-demand + Azure — no credentials needed
docker run -p 8080:8080 ghcr.io/jz-wilson/cloud-price-exporter:latest \
  -regions us-east-1 -lifecycle ondemand -azure-regions eastus

# With AWS credentials for spot pricing
docker run -p 8080:8080 \
  -e AWS_ACCESS_KEY_ID=... \
  -e AWS_SECRET_ACCESS_KEY=... \
  ghcr.io/jz-wilson/cloud-price-exporter:latest \
  -regions us-east-1 -lifecycle spot,ondemand -azure-regions eastus
```

### Helm

```bash
# From GHCR (OCI) — recommended
helm install cloud-price-exporter \
  oci://ghcr.io/jz-wilson/cloud-price-exporter \
  --version 2.1.0 \
  --set exporter.azure.regions=eastus

# From local chart
helm install cloud-price-exporter . \
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
| `-regions` | *(all)* | Comma-separated AWS regions. Empty = auto-discovers all (requires credentials) |
| `-lifecycle` | `spot,ondemand` | Comma-separated lifecycle types: `spot`, `ondemand` |
| `-product-descriptions` | `Linux/UNIX` | Spot instance OS filter: `Linux/UNIX`, `SUSE Linux`, `Windows`, and `(Amazon VPC)` variants |
| `-operating-systems` | `Linux` | On-demand OS filter: `Linux`, `RHEL`, `SUSE`, `Windows` |
| `-saving-plan-types` | *(none)* | Comma-separated savings plan types: `Compute`, `EC2Instance`, `SageMaker` |

**IAM permissions required only for spot pricing and savings plans:**

```json
{
  "Effect": "Allow",
  "Action": [
    "ec2:DescribeSpotPriceHistory",
    "ec2:DescribeAvailabilityZones",
    "ec2:DescribeRegions",
    "savingsplans:DescribeSavingsPlansOfferingRates"
  ],
  "Resource": "*"
}
```

On-demand pricing and instance metadata (vCPU/memory) require **no IAM permissions** — fetched from public APIs.

### Azure Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-azure-enabled` | `true` | Enable Azure VM pricing |
| `-azure-regions` | *(empty)* | Comma-separated Azure regions (e.g. `eastus`, `westeurope`). Empty = skipped |
| `-azure-operating-systems` | `Linux` | Comma-separated OS types: `Linux`, `Windows` |
| `-azure-instance-regexes` | `.*` | Comma-separated regexes to filter Azure VM SKU names |

Azure requires **no credentials** — the Retail Prices API is public.

## Operating Modes

Both providers are enabled by default. Disable either with `-aws-enabled=false` or `-azure-enabled=false`.

| Mode | Command |
|---|---|
| Both clouds | `go run . -regions us-east-1 -azure-regions eastus` |
| AWS only | `go run . -azure-enabled=false -regions us-east-1` |
| Azure only | `go run . -aws-enabled=false -azure-regions eastus` |
| Credential-free | `go run . -lifecycle ondemand -azure-regions eastus` |

## Helm Chart Configuration

All exporter flags map to `values.yaml`:

```yaml
exporter:
  cache: 300
  instanceRegexes: ""
  logLevel: "info"

  aws:
    enabled: true
    regions: ""                    # Empty = auto-discover all (requires credentials)
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

AWS on-demand only (no credentials needed):

```yaml
exporter:
  aws:
    regions: "us-east-1"
    lifecycle: "ondemand"
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

Use [IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) (IAM Roles for Service Accounts) when spot pricing or savings plans are enabled:

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
main.go                              CLI flags, config parsing, HTTP server
exporter/
  exporter.go                        Exporter struct, Prometheus Collector interface, scrape orchestration
  aws/
    clients.go                       AWS client interfaces (EC2Client, ClientFactory)
    factory.go                       Production AWS SDK client factory
    instances.go                     Instance metadata (vCPU/memory) — fetched from ec2instances.info
    ondemand.go                      AWS on-demand pricing — fetched from AWS public bulk pricing URL
    spot.go                          AWS spot pricing (requires IAM credentials)
    savingplan.go                    AWS savings plan pricing (requires IAM credentials)
    types.go                         AWS response types and constants
  azure/
    clients.go                       Azure client interfaces
    retail_client.go                 Azure HTTP client (Retail Prices API)
    ondemand.go                      Azure VM on-demand pricing scraper
    types.go                         Azure Retail Prices API response types
  provider/
    provider.go                      Shared ScrapeResult type and helpers
```

### How Scraping Works

1. Prometheus calls `Collect()` on the exporter
2. If the cache has expired, a new scrape begins
3. Each AWS region and Azure region spawns a concurrent goroutine
4. Each goroutine creates its own API clients via the factory pattern (avoids data races)
5. Results stream through a `scrapeResult` channel to `setPricingMetrics()`
6. Metrics are set on the appropriate `prometheus.GaugeVec` by name (`ec2`, `ec2_memory`, `ec2_vcpu`, `azure_vm`)

### Data Sources

| Data | Source | Auth |
|---|---|---|
| AWS on-demand pricing | `pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/{region}/index.json` | None |
| AWS instance vCPU/memory | `ec2instances.info/instances.json` | None |
| AWS spot pricing | `ec2:DescribeSpotPriceHistory` | IAM |
| AWS availability zones | `ec2:DescribeAvailabilityZones` | IAM |
| AWS savings plans | `savingsplans:DescribeSavingsPlansOfferingRates` | IAM |
| Azure VM pricing | `prices.azure.com/api/retail/prices` | None |

## Development

### Run Tests

```bash
# Unit tests — no credentials needed
go test -race ./...

# Integration tests (Azure: no auth; AWS spot/savings: needs credentials)
go test -tags integration -v ./exporter/

# Coverage
go test -cover ./...
```

### Build

```bash
go build -o cloud-price-exporter .
```

### Lint & Helm

```bash
# Go lint
golangci-lint run ./...

# Helm
helm lint .
helm template test . > /dev/null
```

### CI/CD

GitHub Actions runs on every push and pull request:

- **Test** — `go vet` + `go test -race ./...`
- **Lint** — `golangci-lint`
- **Helm** — `helm lint` + `helm template`
- **Docker** — build image (no push on PRs)

Pushing a `v*` tag triggers the release workflow: builds and pushes the Docker image to [GHCR](https://ghcr.io/jz-wilson/cloud-price-exporter) and creates a GitHub release with auto-generated notes.

```bash
git tag v2.2.0 && git push origin v2.2.0
```
