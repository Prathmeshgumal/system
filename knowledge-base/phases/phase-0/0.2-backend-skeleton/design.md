# Part 0.2 — Backend Skeleton: Design

- **Phase:** 0 — Foundations
- **Status:** 🟦 in design (brainstorming) — decisions in progress
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
| B | Folder layout | **By feature + `internal/platform/`** (config, logging, database, httpserver) | A feature lives in one folder; matches the modular monolith; each feature can become a service later — [Note 4](../../../Notes.md#4-go-packages--project-layout) |
| C | Config | **`caarlos0/env`** struct tags + our own `Validate()`, loaded once at startup in `internal/platform/config` | One line per setting, type conversion + required built in, fail fast — [Note 5](../../../Notes.md#5-configuration-via-environment-variables) |
| D | Logging | **zap** (`zap.Logger`, typed fields): console + DEBUG in dev, JSON + INFO in prod; request-ID + request-logging middleware; replaces `log/slog` from 0.1 | Fast, battle-tested, structured — [Note 6](../../../Notes.md#6-structured-logging--request-ids) |
| E | Routing & middleware | **Standard `net/http`** (`http.ServeMux`, Go 1.22 patterns) + our own middleware (recover → request ID → request log) and a small chain helper, in `internal/platform/httpserver` | No dependency; you write every middleware; all Go code speaks `http.Handler` — [Note 7](../../../Notes.md#7-http-routing--middleware) |
| F | DB access | **sqlc** — SQL in `.sql` files, `sqlc generate` (via `make sqlc`) produces typed Go on top of pgx; generated code is committed | Write real SQL; mistakes caught at generate time; no runtime magic — [Note 8](../../../Notes.md#8-talking-to-the-database-driver-orm-or-sqlc) |

## Sections

*(Added as they are approved.)*
