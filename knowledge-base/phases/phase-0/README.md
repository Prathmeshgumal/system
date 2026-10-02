# Phase 0 — Foundations

**Goal:** a solid, fast development setup that every later part builds on. No chat
features yet — by the end, a React app and a Go API run together, talk to Postgres,
and can run both on your machine and fully in containers.

**Ends with:** the base for v0.1 (Phase 1).

| Part | Build | Learn | Status | Docs |
|---|---|---|---|---|
| 0.1 Dev environment | Monorepo skeleton, docker compose + Postgres, Go `/livez` `/readyz` `/api/status`, React status page, Makefile, Dockerfiles | Containers, multi-stage builds, reverse proxy vs CORS, health checks | 🟦 design | [0.1](0.1-dev-environment/) |
| 0.2 Backend skeleton | Go layout, config, structured logging, DB pool tuning, migrations, sqlc, integration-test setup, full graceful shutdown | Go project structure, connection pooling, migrations, testing against a real DB | ⬜ | — |
| 0.3 Frontend skeleton | Routing, API client, TanStack Query, Tailwind, app layout | Single-page app ↔ API, server state vs UI state | ⬜ | — |

Each part folder holds: `design.md` (spec) · `plan.md` (build plan) · `learnings.md` (results) · `diagrams/`.
