# Every command for the project. Run `make help` to list them.
# Recipe lines must start with a TAB, not spaces.

# Load .env (if present) and export its values to every command below.
-include .env
export

SQLC_IMAGE := sqlc/sqlc:1.30.0

.DEFAULT_GOAL := help
.PHONY: help db migrate migrate-down migrate-status migrate-new sqlc api web dev up-full down clean logs psql test test-unit smoke

help: ## List all commands
	@grep -E '^[a-z-]+:.*## ' $(firstword $(MAKEFILE_LIST)) | awk -F':.*## ' '{printf "  make %-15s %s\n", $$1, $$2}'

db: ## Start only Postgres in Docker
	docker compose up -d --wait postgres

migrate: ## Apply new database migrations
	cd backend && go run ./cmd/api migrate up

migrate-down: ## Undo the last migration
	cd backend && go run ./cmd/api migrate down

migrate-status: ## List migrations and whether they've run
	cd backend && go run ./cmd/api migrate status

migrate-new: ## Create a migration file: make migrate-new name=add_title
	@test -n "$(name)" || (echo "usage: make migrate-new name=add_title" && exit 1)
	cd backend && go run ./cmd/api migrate create $(name)

sqlc: ## Regenerate Go code from SQL queries (never edit the output)
	docker run --rm -u "$$(id -u):$$(id -g)" -v "$(CURDIR)/backend:/src" -w /src $(SQLC_IMAGE) generate

api: ## Run the Go API with live reload
	cd backend && go tool air

web: ## Run the Vite dev server (React)
	cd web && pnpm dev

dev: db migrate ## Postgres + migrations + API + web (Ctrl+C stops API and web)
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

test: ## All tests incl. integration (needs Docker)
	cd backend && go test ./...
	cd web && pnpm test

test-unit: ## Fast tests only, no Docker needed
	cd backend && go test -short ./...
	cd web && pnpm test

smoke: ## End-to-end check of full mode
	./scripts/smoke.sh
