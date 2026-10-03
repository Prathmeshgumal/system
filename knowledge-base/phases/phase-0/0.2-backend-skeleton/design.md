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

## Sections

*(Added as they are approved.)*
