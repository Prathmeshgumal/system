# 0001 — Auth: short-lived JWT access token + rotating refresh token in an httpOnly cookie

- **Status:** accepted
- **Date:** 2026-10-10
- **Phase:** 1 (Part 1.1)

## Context
Every API call and the WebSocket need to know who the caller is. Constraints: one API
server now, several from Phase 3 (so per-request checks shouldn't need shared state);
the web app runs same-origin behind a proxy ([Note 1](../Notes.md#1-same-origin-policy-cors--reverse-proxies));
we want to be able to log a device out; and we want to learn how real apps do it.

## Options considered
1. **Server sessions only** — random session id in a cookie, looked up in Postgres on every
   request. Simple, instantly revocable. But one DB read per request, cookies on every
   request bring CSRF into play for the whole API, and in Phase 3 every server needs the store.
2. **One long-lived JWT** (e.g. 30 days) in localStorage — stateless, but can't be revoked
   and any XSS bug steals it for a month.
3. **Short JWT access token (15 min, in memory) + long refresh token (30 days, httpOnly
   cookie), rotated on use with reuse detection** — stateless checks on the hot path,
   revocation through the refresh session, XSS can't read the long-lived secret.

## Decision
Option 3.

| Piece | Where it lives | Lifetime | Checked by |
|---|---|---|---|
| Access token: HS256 JWT `{sub, sid, aud:"api", iat, exp}` | JS memory only | 15 min | signature + expiry, no DB |
| Refresh token: 256 random bits | `HttpOnly; SameSite=Strict; Path=/api/auth` cookie; DB stores only SHA-256 | 30 days, sliding | `sessions` row |
| WebSocket ticket: HS256 JWT `{sub, aud:"ws"}` | URL query, once | 60 s | signature + expiry |
| Password | bcrypt, cost 12, ≤ 72 bytes | — | — |

- **Rotation:** each refresh replaces the token; the old hash moves to `prev_token_hash`.
  Presenting a `prev_token_hash` again means a copy exists somewhere → the session is revoked.
- **Tabs:** they share the cookie, so the client serialises refreshes with the Web Locks API
  (one in-flight refresh per tab, one tab at a time) — otherwise two tabs refreshing together
  would look like theft.
- **Algorithm pinned** to HS256 in the parser (blocks `alg: none` and algorithm confusion);
  audiences keep a WebSocket ticket from working as an access token and vice versa.
- **Login timing:** unknown emails still run bcrypt against a dummy hash.
- **Bulkhead (added in Part 1.4):** at most GOMAXPROCS − 1 hashes at once; a request queued
  over 10 s gets `503 busy` + `Retry-After`.

## Consequences
- ✅ Authenticated requests cost no DB query; any API server can check them (Phase 3 ready).
- ✅ Logout and theft response work via the session row.
- ❌ After logout an access token stays valid for up to 15 min. Acceptable now; Phase 3's
  "log out everywhere" can add a small revocation list (in Redis) keyed by `sid`.
- ❌ HS256 means every service that checks tokens holds the signing secret. When services
  split out (Phase 3+), revisit RS256/EdDSA so only `auth` can sign.
- ❌ No rate limiting on login yet (Phase 3.4); email enumeration via signup's "already
  registered" stays possible until email verification (Phase 3.3).
- Revisit bcrypt → Argon2id when we next touch password storage.
