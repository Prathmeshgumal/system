# Merging Part 0.2 with Phases 1–2 — what overlaps, and a proposed path

**Your decision.** Written on 2026-10-10 while Phase 2 was being built on
`claude/dreamy-franklin-1jaomb`. Your `part/0.2-backend-skeleton` branch appeared that evening.
Neither branch is in `main` yet. They were built in parallel, so they solve the same
foundation problems differently. Nothing here has been merged; merging your branch wasn't
part of the request.

## Textual conflicts (`git merge-tree`): 9 files

`.env.example`, `Makefile`, `backend/cmd/api/main.go`, `backend/go.mod`, `backend/go.sum`,
`compose.yaml`, `knowledge-base/Notes.md` (0.2 moved it to `notes/phase-0-notes.md`),
`knowledge-base/phases/README.md`, `web/vite.config.ts`.

## The real overlap: the same job done twice

| Concern | Part 0.2 (your approved design) | Phases 1–2 (this branch) |
|---|---|---|
| Config | `internal/platform/config`, `caarlos0/env` tags | `internal/config`, hand-written `Load(getenv)` |
| Logging | zap, logger in context | `log/slog`, `httpx.Logger(ctx)` |
| Request IDs | `httpserver.RequestID` middleware | `httpx.Log` + `Info` (also route + user for metrics) |
| Error format | `{"error":{code,message,fields,request_id}}`, codes `invalid_json`, `validation_failed`, `unavailable`… | same shape; codes `bad_json`, `invalid_input`, plus feature codes |
| HTTP helpers | `httpserver.WriteJSON/WriteError/DecodeJSON` | `httpx.Handle` (handlers return errors), `ReadJSON` |
| Migrations | goose, `internal/platform/database/migrations`, separate `migrate` step | golang-migrate, `backend/migrations`, run at startup |
| DB access | sqlc (generated, committed) | hand-written pgx SQL |
| Test database | Testcontainers, one container per package, `Truncate` | `TEST_DATABASE_URL`, a fresh database per test |
| Pool | tuned + stats in `/api/status` | default pool + Prometheus collector |
| Shutdown | full drain sequence (`/readyz` 503 first) | basic `Shutdown` + closing WebSockets |
| Notes | `knowledge-base/notes/phase-0-notes.md`, notes 1–13 | `knowledge-base/Notes.md`, notes 1–11 (4–11 collide with 0.2's numbers) |

## Proposed path: port Phases 1–2 onto 0.2's platform

Your 0.2 design says *"Part 1.1 starts with a new migration `00002_drop_notes.sql` and deletes
`internal/notes/`"*, so 0.2 was meant to be the base. In order:

1. Merge `main` ← `part/0.2-backend-skeleton` (your PR).
2. On this branch, merge `main`, keeping 0.2's side for every platform file, then port:
   - config fields (JWT, cookies, bcrypt, S3, metrics) into `platform/config` as `env` tags
   - migrations → goose files `00002_drop_notes`, `00003_users_sessions`, `00004_chats`,
     `00005_reactions_replies`, `00006_avatars`; delete golang-migrate
   - stores → sqlc queries per feature (`users/queries.sql`, `auth/queries.sql`, `chat/queries.sql`)
   - `httpx` → `httpserver` (one error helper, 0.2's codes); keep `Info`/`Route` for metrics, with zap
   - tests → `dbtest.Pool` + `Truncate` (tests already create their own users, so this is mechanical)
   - notes 4–11 → `notes/phase-1-notes.md` (14–17) and `notes/phase-2-notes.md` (18–21), links updated
3. Re-run everything: unit, integration, e2e in all three modes, the deploy test, a short soak.

Rough size: the platform swap touches every backend file but changes no behaviour, so the
existing API, e2e and load tests are the safety net. It's best done in one focused session,
committing per step above.

**Alternatives:** keep this branch's foundations and drop 0.2's (fastest, but throws away an
approved design and its notes), or keep both temporarily (not recommended: two config and
migration systems on one database).
