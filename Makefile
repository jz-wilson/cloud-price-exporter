BINARY_NAME := cloud-price-exporter
GO := go

.DEFAULT_GOAL := help

.PHONY: build test test-integration lint fmt vet docker-build clean help

build: ## Build the binary
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-w -s" -o $(BINARY_NAME) .

test: ## Run unit tests with race detection
	$(GO) test -race -count=1 -v ./...

test-integration: ## Run integration tests
	$(GO) test -tags integration -v ./exporter/

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Format code
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

docker-build: ## Build Docker image
	docker build -t $(BINARY_NAME):latest .

clean: ## Remove binary and clear Go caches
	rm -f $(BINARY_NAME)
	$(GO) clean -cache -testcache

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
