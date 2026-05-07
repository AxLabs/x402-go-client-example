# x402-go-client-example Makefile

# Build variables
BINARY_NAME := x402-client
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X github.com/bane-labs-org/x402-go-client-example/internal/version.Version=$(VERSION) \
	-X github.com/bane-labs-org/x402-go-client-example/internal/version.Commit=$(COMMIT) \
	-X github.com/bane-labs-org/x402-go-client-example/internal/version.BuildTime=$(BUILD_TIME)"

# Go variables
GO := go
GOFLAGS := -v
GOCMD := $(GO)

# Directories
BUILD_DIR := ./build
CMD_DIR := ./cmd/client

# Default target
.DEFAULT_GOAL := help

##@ General

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: run
run: ## Run the client (use ARGS="..." to pass arguments)
	@$(GOCMD) run $(LDFLAGS) $(CMD_DIR) $(ARGS)

.PHONY: run-get
run-get: ## Run a GET request example
	@$(GOCMD) run $(LDFLAGS) $(CMD_DIR) get --url http://localhost:8080/paid/hello --verbose

.PHONY: run-inspect
run-inspect: ## Run an inspect command example
	@$(GOCMD) run $(LDFLAGS) $(CMD_DIR) inspect --url http://localhost:8080/paid/hello

##@ Build

.PHONY: build
build: ## Build the client binary
	@mkdir -p $(BUILD_DIR)
	$(GOCMD) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: build-all
build-all: ## Build for multiple platforms
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOCMD) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GOCMD) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOCMD) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOCMD) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GOCMD) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)
	@echo "Built binaries for multiple platforms in $(BUILD_DIR)/"

.PHONY: install
install: ## Install the binary to GOPATH/bin
	$(GOCMD) install $(LDFLAGS) $(CMD_DIR)

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)
	$(GOCMD) clean -cache

##@ Testing

.PHONY: test
test: ## Run all tests
	$(GOCMD) test $(GOFLAGS) ./...

.PHONY: test-short
test-short: ## Run tests in short mode
	$(GOCMD) test -short ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	$(GOCMD) test -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-integration
test-integration: ## Run integration tests
	$(GOCMD) test $(GOFLAGS) ./test/...

.PHONY: test-e2e
test-e2e: ## Run E2E tests (requires running x402-go-server-example)
	$(GOCMD) test ./test/e2e/... -count=1 -v

.PHONY: test-race
test-race: ## Run tests with race detection
	$(GOCMD) test -race ./...

##@ Code Quality

.PHONY: fmt
fmt: ## Format code
	$(GOCMD) fmt ./...
	@echo "Code formatted"

.PHONY: lint
lint: ## Run linter (requires golangci-lint)
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

.PHONY: vet
vet: ## Run go vet
	$(GOCMD) vet ./...

.PHONY: check
check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)

##@ Dependencies

.PHONY: deps
deps: ## Download dependencies
	$(GOCMD) mod download

.PHONY: deps-update
deps-update: ## Update dependencies
	$(GOCMD) get -u ./...
	$(GOCMD) mod tidy

.PHONY: deps-tidy
deps-tidy: ## Tidy go.mod
	$(GOCMD) mod tidy

.PHONY: deps-verify
deps-verify: ## Verify dependencies
	$(GOCMD) mod verify

##@ Documentation

.PHONY: docs
docs: ## Generate documentation (godoc)
	@echo "Starting godoc server at http://localhost:6060"
	@echo "View package at: http://localhost:6060/pkg/github.com/bane-labs-org/x402-go-client-example/"
	godoc -http=:6060

##@ Version

.PHONY: version
version: ## Show version information
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
