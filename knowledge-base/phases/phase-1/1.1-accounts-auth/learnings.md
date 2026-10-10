# Part 1.1 — Accounts & auth: Learnings

Spec: [design.md § 3](../design.md#section-3--part-11-accounts--auth) · Decision: [ADR 0001](../../../adr/0001-auth-tokens.md) ·
Concepts: [Note 4](../../../Notes.md#4-sessions-jwts--where-tokens-live), [Note 7](../../../Notes.md#7-password-hashing--cpu-heavy-work)

## What was built

| Piece | Where | Read this first |
|---|---|---|
| Config from env, validated at startup | `backend/internal/config/` | why a short `JWT_SECRET` stops the server |
| Migrations, applied on startup | `backend/migrations/`, `backend/internal/db/` | `0001_users_sessions.up.sql` comments |
| Users: validation, store, profile + search API | `backend/internal/users/` | `searchPattern` (LIKE escaping), `GetMany` |
| Passwords, tokens, sessions | `backend/internal/auth/` | `sessions.go` `Rotate` |
| Auth endpoints + cookie | `backend/internal/auth/handler.go` | `respond` (cookie flags), `login` (same answer for every failure) |
| API client with refresh-once | `web/src/lib/api.ts` | `refreshSession` |
| Login / signup / profile pages | `web/src/auth/`, `web/src/profile/` | `AuthProvider` startup refresh |

Tests: `internal/auth` (passwords, JWT attacks, rotation/replay), `internal/users`, `internal/app/auth_test.go`
(full HTTP flows), `web/src/lib/api.test.ts`, `web/src/auth/LoginPage.test.tsx`, e2e "a reload keeps you logged in".

## Already observed while building

- **Signup takes ~250 ms** (`duration_ms=248.977` in the log) — that's bcrypt cost 12, exactly the estimate.
- **Reloading the page logged users out in dev mode.** The refresh call got `403`: Vite's proxy
  shorthand rewrites `Host` to `localhost:8080`, so our Origin check saw a cross-origin request.
  Fixed with `changeOrigin: false`; the e2e suite now reloads the page. Full write-up in
  [Note 1 → Gotchas](../../../Notes.md#gotchas). Full mode (Caddy) never had the bug.
- `crypto.randomUUID` and `navigator.locks` only exist on "secure" pages (https or localhost).
  A friend opening `http://192.168.x.x:5173` gets neither, so both have fallbacks.

## Experiments to run

### 1. Look inside a token
Log in, copy the `Authorization` header from any `/api` request in DevTools, paste it into
jwt.io. What claims are there? Change one character of the payload and call
`curl -H "Authorization: Bearer <edited>" localhost:8080/api/me` — what happens?

**Your results:**

### 2. Steal a refresh token (on yourself)
1. Log in. In DevTools → Application → Cookies, copy `relay_refresh`.
2. `curl -X POST -b "relay_refresh=<copied>" localhost:8080/api/auth/refresh` — it works, and rotates.
3. Run the same command again with the *same* old value. What status? Now reload the browser tab — are you still logged in? Why?

**Your results:**

### 3. Timing attack
```sh
time curl -s -X POST localhost:8080/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"nobody@example.com","password":"whatever1"}'
time curl -s -X POST localhost:8080/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"<your email>","password":"whatever1"}'
```
Similar times? Comment out the `VerifyNobody` call in `login` and try again.

**Your results:**
