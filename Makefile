# Every command for the project. Run `make help` to list them.
# Recipe lines must start with a TAB, not spaces.

# Load .env (if present) and export its values to every command below.
-include .env
export

.DEFAULT_GOAL := help
.PHONY: help db api web dev up-full down clean logs psql test e2e smoke loadtest

help: ## List all commands
	@grep -hE '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-9s %s\n", $$1, $$2}'

db: ## Start Postgres and object storage in Docker
	docker compose up -d --wait postgres storage

api: ## Run the Go API with live reload
	cd backend && go tool air

web: ## Run the Vite dev server (React)
	cd web && pnpm dev

dev: db ## Postgres + API + web together (Ctrl+C stops API and web)
	$(MAKE) -j2 api web

up-full: ## Build real images and run everything in containers (http://localhost:8000)
	VERSION=$$(git rev-parse --short HEAD) docker compose --profile full up --build

down: ## Stop everything (database data is kept)
	docker compose --profile full down

clean: ## Stop everything and DELETE the database volume
	docker compose --profile full down -v

logs: ## Follow logs of all containers
	docker compose --profile full logs -f

psql: ## Open a SQL shell in the database
	docker compose exec postgres psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

test: ## Run backend (unit + integration, needs `make db`) and frontend tests
	cd backend && go test ./...
	cd web && pnpm test

e2e: ## Browser tests: two users chatting (starts API + Vite unless running)
	cd web && pnpm exec playwright test

smoke: ## End-to-end check of full mode
	./scripts/smoke.sh

loadtest: ## 100 simulated users against the running API (k6, or k6 in Docker)
	@if command -v k6 >/dev/null; then k6 run loadtest/chat.js; \
	else docker run --rm -i --network host grafana/k6 run - < loadtest/chat.js; fi
