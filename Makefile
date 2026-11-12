# GoDDI Makefile

# Go parameters
BINARY_NAME=goddi
GO=go
GOFLAGS=-v
VERSION ?= $(shell sed -nE 's/^[[:space:]]*Version[[:space:]]*=[[:space:]]*"([^"]+)".*/\1/p' cmd/goddi/main.go | head -n 1)
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.GitCommit=$(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.BuildDate=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')"

# Build directory
BUILD_DIR=./dist

.PHONY: all build test lint clean docker-build migrate fmt vet verify verify-race verify-browser verify-tidy verify-dns-drill

all: lint test build

## build: Build the binary
build: web-build
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/goddi
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: web-build
web-build:
	cd web-admin && pnpm install --frozen-lockfile && pnpm build
	rm -rf web/dist && mkdir -p web/dist
	cp -r web-admin/dist/. web/dist/

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

## verify: Full offline verification matrix (Go build/test/vet, frontend lint/typecheck/build, patch hygiene)
# Reproducible form of the evidence in docs/audit/IMPLEMENTATION-STATUS.md §1.
# Browser suites are separate: they need a running server and the tester's
# Playwright harness; see that document for the 15-suite procedure.
# `pnpm lint` runs with --fix, so commit any formatting it applies.
verify:
	@echo "== Go: build =="
	$(GO) build -mod=readonly ./...
	@echo "== Go: tests =="
	$(GO) test -mod=readonly ./...
	@echo "== Go: vet =="
	$(GO) vet -mod=readonly ./...
	@echo "== Go: gofmt =="
	@test -z "$$(gofmt -l cmd internal pkg)" || { echo "unformatted Go files:"; gofmt -l cmd internal pkg; exit 1; }
	@echo "== Frontend: lint =="
	cd web-admin && pnpm lint
	@echo "== Frontend: typecheck =="
	cd web-admin && pnpm typecheck
	@echo "== Frontend: build =="
	cd web-admin && pnpm build
	@echo "== Patch hygiene =="
	git diff --check
	@echo "verify: OK"

## verify-race: Same as verify plus the full Go race detector (slower)
# Race is an additive check; the rest of verify runs once first.
verify-race: verify
	@echo "== Go: race =="
	$(GO) test -mod=readonly -race ./...
	@echo "verify-race: OK"

## verify-tidy: Verify go.mod / go.sum are tidy without modifying them
# -diff prints the diff and exits non-zero when the module files drift.
# A cold module cache needs network access to compute the tidy graph.
verify-tidy:
	@echo "== Go: module hygiene =="
	$(GO) mod tidy -diff
	@echo "verify-tidy: OK"

## verify-e2e: Run the two Playwright suites CI runs (bare + seeded fixture) locally
# Starts its own instances; needs `cd e2e && npm ci` once. See scripts/e2e_local.sh.
verify-e2e:
	@bash scripts/e2e_local.sh

## verify-browser: Run the 20 browser/static suites against a running isolated server (needs GODDI_ADMIN_PASS)
# Requires a running server and Playwright; see test/browser/README.md.
verify-browser:
	@test -n "$(GODDI_ADMIN_PASS)" || { echo "GODDI_ADMIN_PASS is required; see test/browser/README.md" >&2; exit 2; }
	./test/browser/run.sh

## verify-dns-drill: DNS 方案A 主从 loopback 演练（真实 DNS 服务器 + SecondarySync + DNS 查询）
verify-dns-drill:
	@bash scripts/dns_primary_secondary_drill.sh

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
