APP_NAME := apigo-docker
COMPOSE  := docker compose

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

## ---------- Local development ----------

.PHONY: env
env: ## Create .env from .env.example if missing
	@test -f .env || (cp .env.example .env && echo ".env created, set NEWS_API_KEY")

.PHONY: deps
deps: ## Start only Postgres + Redis (for local go run)
	$(COMPOSE) up -d postgres redis

.PHONY: run
run: env deps ## Run app locally (Postgres + Redis via Docker)
	@export $$(grep -v '^#' .env | xargs) && go run .

.PHONY: build
build: ## Build binary to ./bin/server
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/server .

.PHONY: test
test: ## Run tests with race detector
	go test -v -race ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Sync go.mod/go.sum
	go mod tidy

.PHONY: check
check: vet test build ## Run vet + test + build (same as CI)

## ---------- Docker ----------

.PHONY: up
up: env ## Build and start full stack (app + Postgres + Redis)
	$(COMPOSE) up --build -d

.PHONY: down
down: ## Stop stack (data kept in volumes)
	$(COMPOSE) down

.PHONY: down-clean
down-clean: ## Stop stack and DELETE volumes (Postgres + Redis data)
	$(COMPOSE) down -v

.PHONY: logs
logs: ## Tail app logs
	$(COMPOSE) logs -f app

.PHONY: ps
ps: ## Show container status
	$(COMPOSE) ps

.PHONY: restart
restart: ## Rebuild and restart only the app container
	$(COMPOSE) up --build -d app

## ---------- Database / Redis ----------

.PHONY: psql
psql: ## Open psql shell in Postgres container
	$(COMPOSE) exec postgres psql -U postgres -d newsdb

.PHONY: redis-cli
redis-cli: ## Open redis-cli in Redis container
	$(COMPOSE) exec redis redis-cli

## ---------- Smoke test ----------

.PHONY: smoke
smoke: ## Hit health + news endpoints (stack must be up)
	@curl -sf http://localhost:8082/health && echo ""
	@curl -sf http://localhost:8082/news | head -c 200 && echo " ..."
	@curl -sf http://localhost:8082/news/1 | head -c 200 && echo " ..."

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin
