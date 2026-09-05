# GoDDI Makefile

# Go parameters
BINARY_NAME=goddi
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-X main.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo '0.1.0') -X main.GitCommit=$(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.BuildDate=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')"

# Build directory
BUILD_DIR=./dist

.PHONY: all build test lint clean docker-build migrate fmt vet

all: lint test build

## build: Build the binary
build: web-build
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/goddi
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: web-build
web-build:
	cd web && npm ci && npm run build

## test: Run all tests
test:
	@echo "Running tests..."
	$(GO) test -race -cover ./...

## lint: Run golangci-lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	gofmt -w .
	$(GO) mod tidy

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY_NAME)
	$(GO) clean

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t goddi:latest .
	docker tag goddi:latest goddi:$(shell git describe --tags --always --dirty 2>/dev/null || echo '0.1.0')

## migrate: Run database migrations
migrate: build
	@echo "Running migrations..."
	$(BUILD_DIR)/$(BINARY_NAME) migrate --config config.yaml

## run: Run the server locally
run: build
	$(BUILD_DIR)/$(BINARY_NAME) serve --config config.yaml

## dev: Run with hot reload (requires air)
dev:
	air

## tidy: Tidy Go modules
tidy:
	$(GO) mod tidy

## generate: Run go generate
generate:
	$(GO) generate ./...

## help: Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
