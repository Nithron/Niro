SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/nithron/niro/internal/buildinfo.Version=$(VERSION)
COMPOSE := docker compose -f deploy/compose.dev.yml

SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
OAPI := go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.9.0
GOLANGCI := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

.PHONY: fuzz audit deploy-check monitoring-check soak soak-short help dev-deps dev-down run run-api run-bot migrate web-install web-dev web-build \
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

contract-lint: ## Lint the OpenAPI spec and vet the SQL queries
	cd web && pnpm --package=@redocly/cli@2.83.0 dlx redocly lint ../api/openapi.yaml
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 vet

e2e: web-build ## Run the dashboard's Playwright suite (needs: cd web && pnpm exec playwright install chromium)
	cd web && pnpm e2e

test: ## Run all Go tests (integration tests need Docker)
	go test -race ./...

test-short: ## Run Go unit tests only
	go test -race -short ./...

FUZZTIME ?= 30s
FUZZ_TARGETS := ./internal/core/message:FuzzDocument \
	./internal/core/message:FuzzIsEmoji \
	./internal/modules/moderation:FuzzParseDuration \
	./internal/modules/guardian:FuzzExtractLinks \
	./internal/modules/guardian:FuzzNormalizeHost \
	./internal/modules/guardian:FuzzNormalizeText \
	./internal/modules/guardian:FuzzNormalizeDomains \
	./internal/modules/guardian:FuzzParseFeed \
	./internal/modules/tickets:FuzzTranscript \
	./internal/config:FuzzParseBaseURL \
	./internal/api:FuzzClientIP

fuzz: ## Run every fuzz target for FUZZTIME (default 30s each)
	@n=$$(grep -rh '^func Fuzz' --include='*_test.go' internal | wc -l); \
	  [ $$n -eq $(words $(FUZZ_TARGETS)) ] || { echo "FUZZ_TARGETS lists $(words $(FUZZ_TARGETS)) of $$n fuzz targets"; exit 1; }
	@set -e; for t in $(FUZZ_TARGETS); do \
	  echo "== $${t##*:}"; go test -run='^$$' -fuzz="^$${t##*:}$$" -fuzztime=$(FUZZTIME) $${t%%:*}; \
	done

SOAK_DB ?= postgres://niro:niro@localhost:5432/niro_soak?sslmode=disable
SOAK_VALKEY ?= localhost:6379

soak: ## Load test: 2,500 servers, 100 msg/s for 2 hours (SOAK_DB must be a throwaway database)
	go run ./cmd/niro-soak -database "$(SOAK_DB)" -valkey $(SOAK_VALKEY) -guilds 2500 -rate 100 -duration 2h -warmup 5m

soak-short: ## A one-minute load test (CI)
	go run ./cmd/niro-soak -database "$(SOAK_DB)" -valkey $(SOAK_VALKEY) -guilds 200 -rate 50 -duration 60s -warmup 20s -raid-every 20s -report 20s

deploy-check: monitoring-check ## Validate the compose file (with the example settings) and the monitoring config
	@tmp=$$(mktemp -d) && cp -r deploy "$$tmp/" && cp deploy/.env.example "$$tmp/deploy/.env" && \
	  POSTGRES_PASSWORD=x VALKEY_PASSWORD=x NIRO_DOMAIN=example.com docker compose -f "$$tmp/deploy/compose.yml" --profile tls config -q && \
	  echo "deploy/compose.yml is valid"; rc=$$?; rm -rf "$$tmp"; exit $$rc

monitoring-check: ## Validate the alert rules (with their unit tests) and the Alertmanager config
	docker run --rm -v "$(CURDIR)/deploy/monitoring:/etc/prometheus:ro" -w /etc/prometheus --entrypoint promtool prom/prometheus:v3.16.0 check config prometheus.yml
	docker run --rm -v "$(CURDIR)/deploy/monitoring:/m:ro" -w /m --entrypoint promtool prom/prometheus:v3.16.0 test rules alerts_test.yml
	docker run --rm -v "$(CURDIR)/deploy/monitoring:/m:ro" -w /m --entrypoint amtool prom/alertmanager:v0.35.1 check-config alertmanager.yml
	python3 -c "import json,sys; json.load(open('deploy/monitoring/grafana-dashboard.json'))"

audit: ## Check Go and dashboard dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...
	cd web && pnpm audit --prod

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
