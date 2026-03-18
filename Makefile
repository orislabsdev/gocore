# ─────────────────────────────────────────────────────────────────────────────
# gocore Makefile
# ─────────────────────────────────────────────────────────────────────────────
# Usage:
#   make          — runs vet + test
#   make build    — compiles the example binary
#   make test     — runs all tests with race detector
#   make coverage — runs tests and opens an HTML coverage report
#   make lint     — runs golangci-lint (must be installed separately)
#   make clean    — removes build artefacts

MODULE := github.com/orislabsdev/gocore
BINARY := bin/gocore-example
EXAMPLE := ./example

.DEFAULT_GOAL := check

# ─── Core targets ────────────────────────────────────────────────────────────

.PHONY: check
check: vet test ## Run vet + tests (default)

.PHONY: build
build: ## Compile the example binary
	@mkdir -p bin
	go build -ldflags="-s -w" -o $(BINARY) $(EXAMPLE)
	@echo "Built: $(BINARY)"

.PHONY: run
run: ## Run the example server (requires JWT_SECRET env var)
	JWT_SECRET=$${JWT_SECRET:-change-me-in-production} go run $(EXAMPLE)/main.go

.PHONY: test
test: ## Run all tests with the race detector enabled
	go test -race -count=1 ./...

.PHONY: test-verbose
test-verbose: ## Run all tests with verbose output
	go test -race -count=1 -v ./...

.PHONY: coverage
coverage: ## Generate and display an HTML coverage report
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint (install: https://golangci-lint.run)
	golangci-lint run ./...

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	go mod tidy

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf bin coverage.out coverage.html

# ─── Help ─────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
