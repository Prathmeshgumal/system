# Part 0.2 — Backend Skeleton: Learnings

What we measured, what broke, and what you learned. The experiments are **yours to
run**; the results sections are for your own words and numbers.

## What was built — read in this order

| # | Piece | Start here | Note |
|---|---|---|---|
| 1 | Config | [`config.go:19`](../../../../backend/internal/platform/config/config.go#L19) — the `Config` struct | [5](../../../notes/phase-0-notes.md#5-configuration-via-environment-variables) |
| 2 | Logging | [`logging.go:23`](../../../../backend/internal/platform/logging/logging.go#L23) | [6](../../../notes/phase-0-notes.md#6-structured-logging--request-ids) |
| 3 | Middleware | [`middleware.go:23`](../../../../backend/internal/platform/httpserver/middleware.go#L23) — `Chain`, then `Recover`, `RequestID`, `RequestLog` | [7](../../../notes/phase-0-notes.md#7-http-routing--middleware) |
| 4 | Errors | [`errors.go:32`](../../../../backend/internal/platform/httpserver/errors.go#L32) | [13](../../../notes/phase-0-notes.md#13-http-status-codes--api-error-format) |
| 5 | Shutdown | [`server.go:63`](../../../../backend/internal/platform/httpserver/server.go#L63) and its test | [12](../../../notes/phase-0-notes.md#12-graceful-shutdown--connection-draining) |
| 6 | Pool | [`database.go:17`](../../../../backend/internal/platform/database/database.go#L17) | [11](../../../notes/phase-0-notes.md#11-database-connection-pooling) |
| 7 | Migrations | [`00001_create_notes.sql`](../../../../backend/internal/platform/database/migrations/00001_create_notes.sql) → [`migrate.go:28`](../../../../backend/internal/platform/database/migrate.go#L28) | [9](../../../notes/phase-0-notes.md#9-database-migrations) |
| 8 | sqlc | [`queries.sql`](../../../../backend/internal/notes/queries.sql) → [generated code](../../../../backend/internal/notes/db/queries.sql.go#L17) | [8](../../../notes/phase-0-notes.md#8-talking-to-the-database-driver-orm-or-sqlc) |
| 9 | The `notes` slice | [`handler.go`](../../../../backend/internal/notes/handler.go) → [`service.go`](../../../../backend/internal/notes/service.go) → [`store.go`](../../../../backend/internal/notes/store.go) | [4](../../../notes/phase-0-notes.md#4-go-packages--project-layout) |
| 10 | Test DB | [`dbtest.go:56`](../../../../backend/internal/platform/database/dbtest/dbtest.go#L56) | [10](../../../notes/phase-0-notes.md#10-unit-vs-integration-tests-testcontainers) |
| 11 | Wiring it all | [`main.go`](../../../../backend/cmd/api/main.go) — `serve()` reads top to bottom like the startup list in the design | — |

Tests: `make test-unit` (fast) · `make test` (incl. integration) · `make smoke` (full mode end to end).

## Already observed while building

- **Config errors are readable:** missing `DATABASE_URL` → `required environment variable "DATABASE_URL" is not set`;
  `SHUTDOWN_TIMEOUT=abc` → `SHUTDOWN_TIMEOUT has an invalid value…`; exit code 1 before anything starts.
- **Full-mode shutdown took 5.2 s** (`docker compose stop api`): `shutdown started` → 5 s drain →
  `draining in-flight requests` → `http server stopped` → `database pool closed` → `stopped cleanly`.
- **Health-check noise:** Docker's healthcheck logged a `/livez` line every 5 s at info level → probes now log at debug.
- **Bug: 500 instead of 503 when Postgres stopped.** Stopping Postgres kills idle pooled
  connections; the next query fails with `57P01 terminating connection due to administrator
  command`, which the first version didn't recognise as "unavailable". Fixed with
  `database.IsUnavailable` + a test of 10 error kinds. Lesson: test failure modes against the
  real thing — fakes only return the errors you imagined.
- **Integration test cost:** the first test in a package takes ~2–6 s (container start), the rest
  milliseconds. `-short` skips them cleanly.
- **sqlc caught a type choice:** `timestamptz` became `pgtype.Timestamptz` until we told sqlc to use `time.Time`.

## Experiments to run

### 1. Break the config
1. In `.env`, comment out `DATABASE_URL`, run `make api`. Read the error.
2. Restore it, set `DB_MIN_CONNS=20`. Run again. Is it clear what to fix?
3. Restore.

**Your results:**

### 2. Follow one request
1. `make dev`. In another terminal: `curl -i -X POST localhost:8080/api/notes -H 'Content-Type: application/json' -d '{"body":"trace me"}'`
2. Copy `X-Request-ID` from the response; find every log line with that ID in the API's terminal.
3. Set `LOG_FORMAT=json` in `.env`, restart `make api`, repeat. Which format would a log *tool* prefer? Which do *you* prefer reading?

**Your results:**

### 3. Migration lifecycle
1. `make migrate-new name=add_title` → open the new file, add:
   `-- +goose Up` / `ALTER TABLE notes ADD COLUMN title text;` / `-- +goose Down` / `ALTER TABLE notes DROP COLUMN title;`
2. `make migrate`, then `make psql` → `\d notes` and `SELECT * FROM goose_db_version;`
3. `make migrate-down`, check `\d notes` again, then delete the file.

**Your results:**

### 4. sqlc catches a mistake
1. In `backend/internal/notes/queries.sql`, change `body` to `bodyy` in `ListLatestNotes`.
2. `make sqlc` — read the error. Undo the change, `make sqlc` again.

**Your results:**

### 5. Watch the pool
1. Set `DB_MAX_CONNS=1` and `DB_MIN_CONNS=1` in `.env`; `make dev`; open the status page.
2. Fire 50 requests at once:
   `for i in $(seq 50); do curl -s -o /dev/null localhost:8080/api/notes & done; wait`
3. Watch **Pool** on the status page. What does `waited` do? Why?
4. Remove both settings.

**Your results:**

### 6. Graceful shutdown
1. `make up-full`; in another terminal `docker compose logs -f api`.
2. `docker compose stop api` — time it, and read the log sequence. Match each line to the 6 steps in Note 12.

**Your results:**

## Questions to answer for yourself

- Why does the `CHECK` constraint exist if the service already validates the body?
- Why is `Recover` the *outermost* middleware? What breaks if it's innermost?
- Why does `api` wait for `migrate` to **complete**, but `migrate` waits for Postgres to be **healthy**?
- What would happen with 3 API copies × `DB_MAX_CONNS=50` against Postgres's default limit?
