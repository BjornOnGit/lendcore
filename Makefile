.DEFAULT_GOAL := help
.PHONY: help test test-integration lint fmt tidy up-db down reset-db psql

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

# --- Local infrastructure (Docker Compose) ---

DB ?= postgres

up-db: ## Start Postgres and wait until it is healthy
	@echo "==> docker compose up -d --wait postgres"
	@docker compose up -d --wait postgres

down: ## Stop the local stack (data is kept)
	@echo "==> docker compose down"
	@docker compose down

reset-db: ## Stop the local stack and DELETE its data volumes
	@echo "==> docker compose down -v"
	@docker compose down -v

psql: ## Open psql in a database (make psql DB=ledger)
	@echo "==> psql -d $(DB)"
	@docker compose exec postgres psql -U postgres -d "$(DB)"