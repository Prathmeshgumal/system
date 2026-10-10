# Phase 1 — Chat MVP (v0.1): Build Plan

Spec: [design.md](design.md). Every task is **test first**: write the failing test, watch it
fail, write the code, watch it pass, commit. One commit per task (small, readable diffs).

Integration tests need a Postgres: `make db` (Docker) or any local Postgres, then
`TEST_DATABASE_URL` in `.env`. Each test gets its own fresh database, so tests can't leak
into each other or into your dev data.

## Step 0 — Foundations 🔹 (subset of Parts 0.2 / 0.3)

| # | Task | Files | Tests |
|---|---|---|---|
| 0.1 | Typed config from env, validated at startup (`PORT`, `DATABASE_URL`, `JWT_SECRET`, `COOKIE_SECURE`, `BCRYPT_COST`, `PPROF_ADDR`) | `internal/config/` | missing/short `JWT_SECRET` → error; defaults applied |
| 0.2 | Migrations embedded with `go:embed`, applied on startup; `api migrate` subcommand | `migrations/`, `internal/db/` | applying twice is a no-op |
| 0.3 | Test helper: create a uniquely-named database, migrate it, drop it on cleanup | `internal/testdb/` | used by every integration test |
| 0.4 | `httpx`: `WriteJSON`, `ReadJSON` (size limit, unknown fields rejected), error format `{"error":{"code","message","fields"}}`, request-log + recover middleware | `internal/httpx/` | bad JSON → 400; panic → 500 and server survives |
| 0.5 | Frontend: Tailwind v4, React Router, TanStack Query; status page moves to `/status` | `web/` | existing status tests still pass |

## Step 1 — Part 1.1 Accounts & auth

| # | Task | Files | Tests |
|---|---|---|---|
| 1.1 | Migration `0001_users_sessions` | `migrations/` | — |
| 1.2 | `users`: validation (email, username, display name, bio) | `internal/users/validate.go` | table-driven valid/invalid cases |
| 1.3 | `users.Store`: Create, GetByID, GetByEmail, GetMany, Search(prefix), Update; unique-violation → `ErrEmailTaken` / `ErrUsernameTaken` | `internal/users/store.go` | integration |
| 1.4 | `auth`: password hash/verify (bcrypt, 72-byte limit, dummy-hash on unknown email) | `internal/auth/password.go` | unit |
| 1.5 | `auth`: access JWT + WebSocket ticket issue/parse (alg pinned to HS256, `aud` checked, expiry) | `internal/auth/tokens.go` | expired, wrong secret, wrong audience, `alg:none` rejected |
| 1.6 | `auth.Sessions`: create, rotate, reuse detection, revoke | `internal/auth/sessions.go` | integration: rotate → replay old → session revoked |
| 1.7 | Auth HTTP: signup, login, refresh, logout, cookie, Origin check; `RequireUser` middleware | `internal/auth/handler.go`, `middleware.go` | integration via `httptest` |
| 1.8 | Users HTTP: `GET/PATCH /api/me`, `GET /api/users?q=`, `GET /api/users/{id}` | `internal/users/handler.go` | integration |
| 1.9 | Wire into `main.go` | `cmd/api/main.go` | — |
| 1.10 | Web: API client (bearer, refresh-once, shared in-flight refresh, Web Locks), `AuthProvider`, `RequireAuth` | `web/src/lib/api.ts`, `web/src/auth/` | 401 → one refresh → retry; refresh fails → logged out |
| 1.11 | Web: login, signup, profile pages | `web/src/auth/`, `web/src/profile/` | form validation + error display |

## Step 2 — Part 1.2 Chats over REST

| # | Task | Files | Tests |
|---|---|---|---|
| 2.1 | Migration `0002_chats` (chats, chat_members, messages + indexes) | `migrations/` | — |
| 2.2 | `chat.Store`: direct find-or-create, create group, get chat, list my chats (one query, lateral last message, unread) | `internal/chat/store.go` | integration |
| 2.3 | `chat.Store`: members — add, remove/leave (last-admin promotion), set role, rename | same | integration: admin rules |
| 2.4 | `chat.Store`: send (seq under row lock, idempotent `client_id`), edit, delete, history `before`/`after`, mark read | same | integration: 20 concurrent sends → seq 1…20; retries return same message |
| 2.5 | `chat.Service`: membership/admin checks, validation, batch-load users (`users.GetMany`), publish events after commit | `internal/chat/service.go` | unit with fakes for `Publisher` |
| 2.6 | Chat HTTP handlers | `internal/chat/handler.go` | integration via `httptest` |
| 2.7 | Web: app layout, chat list, new chat / new group dialogs | `web/src/chat/` | chat list renders previews + unread |
| 2.8 | Web: chat view — infinite scroll up, composer with optimistic send, edit/delete | `web/src/chat/` | cache helpers unit-tested |
| 2.9 | Web: group info panel — members, add/remove/promote, rename, leave | `web/src/chat/` | — |

## Step 3 — Part 1.3 Real-time

| # | Task | Files | Tests |
|---|---|---|---|
| 3.1 | `realtime.Hub`: register/unregister, `Publish(userIDs, event)`, bounded outbox, slow-consumer eviction | `internal/realtime/hub.go` | unit: fan-out to all tabs, full outbox → closed, no blocking |
| 3.2 | `POST /api/ws/ticket`, `GET /ws` handler: ticket + Origin check, writer loop, ping/pong, cleared write deadline | `internal/realtime/handler.go` | integration: real WS client gets `message.created` after REST send; bad ticket → 401 |
| 3.3 | Proxies: Vite `/ws` (`ws: true`), Caddy `handle /ws` | `web/vite.config.ts`, `web/Caddyfile` | smoke test |
| 3.4 | Web: socket client — connect with ticket, backoff + jitter, status indicator | `web/src/realtime/` | backoff schedule unit test |
| 3.5 | Web: events → cache updates (append/replace/tombstone, chat list preview + unread, invalidate on structural events), catch-up on reconnect, mark-read when chat is open and visible | `web/src/realtime/`, `web/src/chat/cache.ts` | pure-function tests for every event |
| 3.6 | E2E: two Chromium users chat live (`scripts/e2e.sh`, Playwright) | `web/e2e/`, `scripts/` | the test itself |

## Step 4 — Part 1.4 Measure it

| # | Task | Files |
|---|---|---|
| 4.1 | pprof on `PPROF_ADDR` (private, off by default) | `cmd/api/main.go` |
| 4.2 | k6 script: setup users + groups, 100 VUs, WebSocket delivery-latency trend, thresholds; `make loadtest` | `loadtest/chat.js`, `Makefile` |
| 4.3 | Run it, profile it, find the first bottleneck, fix it, re-run, compare | — |
| 4.4 | Write up numbers in `1.4-measure/learnings.md` | knowledge-base |

## Step 5 — Docs

- ADRs 0001 (auth), 0002 (schema), 0003 (WebSocket protocol)
- `Notes.md`: 4 Sessions & JWTs, 5 HTTP vs polling vs WebSockets, 6 Ordering & cursor pagination, 7 Password hashing
- `phases/README.md` + `phase-1/README.md` tracker; per-part `learnings.md` with experiments to run
- `.env.example`, `compose.yaml` (new env vars), `README.md` quick start
