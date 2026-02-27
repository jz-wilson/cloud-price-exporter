BINARY_NAME := cloud-price-exporter
GO := go

.DEFAULT_GOAL := help

.PHONY: build test test-integration lint fmt vet docker-build clean help helm-template helm-lint bump-major bump-minor bump-patch

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

helm-template: ## Render chart templates locally
	helm template test .

helm-lint: ## Lint the Helm chart
	helm lint .

bump-major: ## Bump major version (X.0.0)
	@VERSION=$$(grep '^version:' Chart.yaml | awk '{print $$2}'); \
	MAJOR=$$(echo $$VERSION | cut -d. -f1); \
	MINOR=$$(echo $$VERSION | cut -d. -f2); \
	PATCH=$$(echo $$VERSION | cut -d. -f3); \
	NEW="$$((MAJOR + 1)).0.0"; \
	sed -i '' "s/^version: .*/version: $$NEW/" Chart.yaml; \
	sed -i '' "s/^next-version: .*/next-version: $$NEW/" GitVersion.yml; \
	echo "$$VERSION -> $$NEW"

bump-minor: ## Bump minor version (x.Y.0)
	@VERSION=$$(grep '^version:' Chart.yaml | awk '{print $$2}'); \
	MAJOR=$$(echo $$VERSION | cut -d. -f1); \
	MINOR=$$(echo $$VERSION | cut -d. -f2); \
	PATCH=$$(echo $$VERSION | cut -d. -f3); \
	NEW="$$MAJOR.$$((MINOR + 1)).0"; \
	sed -i '' "s/^version: .*/version: $$NEW/" Chart.yaml; \
	sed -i '' "s/^next-version: .*/next-version: $$NEW/" GitVersion.yml; \
	echo "$$VERSION -> $$NEW"

bump-patch: ## Bump patch version (x.y.Z)
	@VERSION=$$(grep '^version:' Chart.yaml | awk '{print $$2}'); \
	MAJOR=$$(echo $$VERSION | cut -d. -f1); \
	MINOR=$$(echo $$VERSION | cut -d. -f2); \
	PATCH=$$(echo $$VERSION | cut -d. -f3); \
	NEW="$$MAJOR.$$MINOR.$$((PATCH + 1))"; \
	sed -i '' "s/^version: .*/version: $$NEW/" Chart.yaml; \
	sed -i '' "s/^next-version: .*/next-version: $$NEW/" GitVersion.yml; \
	echo "$$VERSION -> $$NEW"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
