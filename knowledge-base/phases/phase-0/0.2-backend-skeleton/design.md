# Part 0.2 — Backend Skeleton: Design

- **Phase:** 0 — Foundations
- **Status:** 🟦 in design (brainstorming) — decisions A–M made; Section 1 approved, Section 2 in review
- **Branch:** `part/0.2-backend-skeleton`

## Goal

Give the Go backend the plumbing every real backend has, so Part 1.1 (accounts) can
focus on the feature itself: config, structured logging, database pool tuning,
migrations, sqlc, integration tests against a real Postgres, and the full graceful
shutdown sequence.

## Decisions

| # | Decision | Chosen | Why |
|---|---|---|---|
| A | Scope of 0.2 | **All 7 pieces now**, practised on a small demo table (removed or replaced later) | Part 1.1 starts with all tooling ready; you learn the tools before the feature |
| B | Folder layout | **By feature + `internal/platform/`** (config, logging, database, httpserver) | A feature lives in one folder; matches the modular monolith; each feature can become a service later — [Note 4](../../../notes/phase-0-notes.md#4-go-packages--project-layout) |
| C | Config | **`caarlos0/env`** struct tags + our own `Validate()`, loaded once at startup in `internal/platform/config` | One line per setting, type conversion + required built in, fail fast — [Note 5](../../../notes/phase-0-notes.md#5-configuration-via-environment-variables) |
| D | Logging | **zap** (`zap.Logger`, typed fields): console + DEBUG in dev, JSON + INFO in prod; request-ID + request-logging middleware; replaces `log/slog` from 0.1 | Fast, battle-tested, structured — [Note 6](../../../notes/phase-0-notes.md#6-structured-logging--request-ids) |
| E | Routing & middleware | **Standard `net/http`** (`http.ServeMux`, Go 1.22 patterns) + our own middleware (recover → request ID → request log) and a small chain helper, in `internal/platform/httpserver` | No dependency; you write every middleware; all Go code speaks `http.Handler` — [Note 7](../../../notes/phase-0-notes.md#7-http-routing--middleware) |
| F | DB access | **sqlc** — SQL in `.sql` files, `sqlc generate` (via `make sqlc`) produces typed Go on top of pgx; generated code is committed | Write real SQL; mistakes caught at generate time; no runtime magic — [Note 8](../../../notes/phase-0-notes.md#8-talking-to-the-database-driver-orm-or-sqlc) |
| G | Migration tool | **goose** — one `.sql` file per migration with `-- +goose Up` / `-- +goose Down`; also used as a Go library; sqlc reads the same files | Up/down side by side; CLI + library; sqlc support — [Note 9](../../../notes/phase-0-notes.md#9-database-migrations) |
| H | When migrations run | **Separate step before the app**: `api migrate` subcommand (migrations embedded in the binary); `make migrate`, run automatically by `make dev`; one-off `migrate` service in Compose that `api` waits for (`service_completed_successfully`) | One run per deploy, no race between copies, standard production pattern — [Note 9](../../../notes/phase-0-notes.md#when-migrations-run-decision-h) |
| I | Integration-test DB | **Testcontainers** (`testcontainers-go`): fresh `postgres:18` per test package, goose migrations applied, container removed afterwards; each test owns its rows | Fully isolated, never touches dev data, `go test` just works — [Note 10](../../../notes/phase-0-notes.md#10-unit-vs-integration-tests-testcontainers) |
| J | Connection pool | **Explicit, configurable pool settings** (max 10, min 2, lifetime 1 h, idle 30 min) + **live pool stats in `/api/status`** (in use, idle, total, waits) | Learn every knob; numbers visible for the Part 1.4 load test; PgBouncer waits for Phase 3 — [Note 11](../../../notes/phase-0-notes.md#11-database-connection-pooling) |
| K | Graceful shutdown | **Full sequence:** `/readyz` → 503, wait `SHUTDOWN_DRAIN_DELAY` (0 s dev / ~5 s prod), `srv.Shutdown` (≤ `SHUTDOWN_TIMEOUT` 10 s), close pool, flush logs, exit; second signal exits immediately; covered by a test | Zero dropped requests during deploys behind a load balancer — [Note 12](../../../notes/phase-0-notes.md#12-graceful-shutdown--connection-draining) |
| L | Demo table | **Throwaway `notes` feature, full slice**: `notes (id, body, created_at)` → goose migration → sqlc queries → store → service → handler; `POST /api/notes`, `GET /api/notes` (latest 20); integration + handler tests. Dropped by a new migration at the start of Part 1.1 | Practise the exact handler → service → store pattern every feature uses, on something with no tricky rules — [Note 4](../../../notes/phase-0-notes.md#4-go-packages--project-layout) |
| M | Error responses | **Structured error object** `{"error":{"code","message","fields","request_id"}}` with stable snake_case codes; used by all endpoints, 404s and the recover middleware (`/api/status` and probes keep their own bodies) | One shape → one frontend error handler; field errors for forms; no internals leaked — [Note 13](../../../notes/phase-0-notes.md#13-http-status-codes--api-error-format) |

## Section 1 — Folder layout ✅ approved

```
backend/
├── cmd/api/main.go              ← load config → logger → db → server → wait for signal
│                                   subcommands: (none) = serve · migrate · healthcheck
├── internal/
│   ├── platform/                ← shared plumbing, knows nothing about features (B)
│   │   ├── config/              Config struct + Load() + Validate()          (C)
│   │   ├── logging/             zap: console+DEBUG dev / JSON+INFO prod      (D)
│   │   ├── database/            pool from config, pool stats, Migrate()      (J, H)
│   │   │   ├── migrations/      00001_create_notes.sql … (embedded)          (G)
│   │   │   └── dbtest/          Testcontainers helper for integration tests  (I)
│   │   └── httpserver/          server, middleware, error helper, shutdown   (E, M, K)
│   ├── health/                  from 0.1; now also reports pool stats        (J)
│   └── notes/                   throwaway demo feature                       (L)
│       ├── queries.sql          SQL for sqlc
│       ├── db/                  generated by sqlc — never edited             (F)
│       ├── store.go             wraps generated queries
│       ├── service.go           rules (body 1–1000 chars)
│       └── handler.go           POST /api/notes, GET /api/notes
└── sqlc.yaml                    where sqlc finds schema + queries
```

- **Each feature owns its queries** (`notes/queries.sql` → `notes/db/`).
- **Migrations are shared infrastructure** in `platform/database/migrations/`, embedded in the binary.
- **New make targets:** `migrate`, `migrate-new name=…`, `migrate-down`, `sqlc`, `test-unit` (no Docker).
- **sqlc and goose pinned as Go tools** (`go tool sqlc`, `go tool goose`), like `air`.

## Section 2 — Startup, config & request flow ⏳ in review
