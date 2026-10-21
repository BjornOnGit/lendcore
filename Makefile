.DEFAULT_GOAL := help
.PHONY: help test test-integration lint fmt tidy

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-18s %s\n", $$1, $$2}'

test: ## Run unit tests (no external services)
	@echo "==> go test -race ./..."
	@go test -race ./...

test-integration: ## Run unit + integration tests (needs Postgres)
	@echo "==> go test -race -tags=integration ./..."
	@go test -race -tags=integration ./...

lint: ## Run golangci-lint
	@echo "==> golangci-lint run ./..."
	@golangci-lint run ./...

fmt: ## Format Go code
	@echo "==> go fmt ./..."
	@go fmt ./...

tidy: ## Tidy go.mod and go.sum
	@echo "==> go mod tidy"
	@go mod tidy