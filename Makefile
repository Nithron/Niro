SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/nithron/niro/internal/buildinfo.Version=$(VERSION)
COMPOSE := docker compose -f deploy/compose.dev.yml

SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
OAPI := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.9.0
GOLANGCI := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

.PHONY: help dev-deps dev-down run run-api run-bot migrate web-install web-dev web-build \
        generate generate-check test test-short lint build docker clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

dev-deps: ## Start Postgres and Valkey in Docker
	$(COMPOSE) up -d --wait

dev-down: ## Stop the development dependencies (data is kept)
	$(COMPOSE) down

run: web-build ## Build the dashboard, then run bot + API + dashboard on :8080
	go run -ldflags "$(LDFLAGS)" ./cmd/niro all

run-api: ## Run only the API (use with `make web-dev`)
	go run ./cmd/niro api

run-bot: ## Run only the bot
	go run ./cmd/niro bot

migrate: ## Apply database migrations
	go run ./cmd/niro migrate

web-install: ## Install dashboard dependencies
	cd web && pnpm install --frozen-lockfile

web-dev: ## Dashboard dev server with hot reload on :5173 (proxies /api and /auth to :8080)
	cd web && pnpm dev

web-build: ## Build the dashboard into web/dist (embedded in the binary)
	cd web && pnpm install --frozen-lockfile && pnpm build

generate: ## Regenerate sqlc, OpenAPI server and TypeScript client code
	$(SQLC) generate
	$(OAPI) -config api/oapi-codegen.yaml api/openapi.yaml
	cd web && pnpm gen:api

generate-check: generate ## Fail if generated code is out of date
	@git diff --exit-code -- internal/store/dbq internal/api/apigen web/src/api/schema.d.ts \
		|| (echo "Generated code is stale: run 'make generate' and commit." && exit 1)

test: ## Run all Go tests (integration tests need Docker)
	go test -race ./...

test-short: ## Run Go unit tests only
	go test -race -short ./...

lint: ## Run golangci-lint and the dashboard linters
	$(GOLANGCI) run ./...
	cd web && pnpm lint && pnpm typecheck

build: web-build ## Build the niro binary into bin/
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/niro ./cmd/niro

docker: ## Build the container image
	docker build -f deploy/Dockerfile --build-arg VERSION=$(VERSION) -t niro:$(VERSION) .

clean: ## Remove build output
	rm -rf bin web/node_modules
	find web/dist -mindepth 1 ! -name .gitkeep -delete 2>/dev/null || true
