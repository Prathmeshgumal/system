# Phase 1 — Chat MVP (v0.1): Design

- **Phase:** 1 — Chat MVP for 100 users
- **Status:** 🟨 building — all four parts in one pass (see [plan.md](plan.md))
- **Branch:** `claude/dreamy-franklin-1jaomb` (off `main` after Part 0.1)
- **Plan:** [plan.md](plan.md) · **ADRs:** [0001 auth](../../adr/0001-auth-tokens.md) ·
  [0002 schema](../../adr/0002-chat-schema.md) · [0003 WebSocket protocol](../../adr/0003-websocket-protocol.md)

## Goal

Two people in two browsers sign up, find each other, start a 1:1 chat or a group, and
messages appear for everyone **live** (< 200 ms locally). History scrolls back forever,
edits and deletes show up live, unread counts are right, and a dropped connection heals
itself and fetches what it missed. A k6 test with **100 concurrent users** passes with no
errors, and we know where the first bottleneck is.

Done-when list: [Section 7](#section-7--testing--done-when).

**Out of scope (later phases):** typing, read receipts, reactions, replies (2.1) ·
pictures (2.2) · AWS (2.3) · metrics dashboards (2.4) · rate limiting, email verification,
password reset, presence, multiple API servers (3.x).

### About Phase 0 parts 0.2 and 0.3

Phase 1 needs a few things that were planned for 0.2 / 0.3 (migrations, config, router,
TanStack Query, Tailwind). Rather than block on them, Phase 1 builds **exactly the subset
it needs**, marked 🔹 below. What's left of 0.2 / 0.3 afterwards (sqlc, pool tuning,
full graceful-shutdown sequence, Playwright) stays in those parts.

---

## Decisions

Chosen by Claude, overridable — each has a "why" and, where it's a real trade-off, an ADR.

| # | Decision | Chosen | Why | Where |
|---|---|---|---|---|
| A | Password hashing | **bcrypt, cost 12** (`golang.org/x/crypto/bcrypt`) | Roadmap pick; slow on purpose; cost tunable. Argon2id is the modern default — noted as the upgrade | [ADR 0001](../../adr/0001-auth-tokens.md) |
| B | Session shape | **Short-lived JWT access token (15 min) + opaque refresh token (30 days) in an httpOnly cookie, rotated on every use, with reuse detection** | Stateless checks on every request (no DB hit), but still revocable via the refresh session | [ADR 0001](../../adr/0001-auth-tokens.md) |
| C | Where the browser keeps tokens | **Access token in memory only; refresh token in `HttpOnly; SameSite=Strict; Path=/api/auth` cookie** | XSS can't read the refresh token; CSRF can't send it cross-site; the access token isn't a cookie so CSRF doesn't apply to the rest of the API | [Note 4](../../Notes.md#4-sessions-jwts--where-tokens-live) |
| D | IDs | **`bigint` identity everywhere** | Simple, small, ordered. Enumerable — acceptable now; Phase 7 revisits (Snowflake IDs) | [ADR 0002](../../adr/0002-chat-schema.md) |
| E | Message ordering | **Per-chat sequence number `seq`**, assigned by bumping `chats.last_seq` inside the insert transaction | Gapless, commit-ordered per chat → safe cursors and safe catch-up. A global `bigserial` is *not* commit-ordered | [ADR 0002](../../adr/0002-chat-schema.md), [Note 6](../../Notes.md#6-ordering--cursor-pagination) |
| F | History pagination | **Cursor on `seq`** (`?before=` for scrolling up, `?after=` for catch-up), never `OFFSET` | `OFFSET` gets slower the deeper you scroll and skips/duplicates rows when new messages arrive | [Note 6](../../Notes.md#6-ordering--cursor-pagination) |
| G | Unread counts | **`chats.last_seq − chat_members.last_read_seq`** — no `COUNT(*)` | O(1) per chat; the chat list stays one query no matter how big chats get | [ADR 0002](../../adr/0002-chat-schema.md) |
| H | Real-time transport | **WebSocket, server → client push only. Sending stays a REST `POST`** | One write path (REST: validation, status codes, idempotency, tests) and one read path (push). Slack worked this way for years | [ADR 0003](../../adr/0003-websocket-protocol.md), [Note 5](../../Notes.md#5-http-vs-polling-vs-websockets) |
| I | WebSocket auth | **Short-lived (60 s) signed ticket** from `POST /api/ws/ticket`, passed as `?ticket=` | Browsers can't set `Authorization` on a WebSocket; a long-lived token in a URL ends up in logs | [ADR 0003](../../adr/0003-websocket-protocol.md) |
| J | Idempotent sends | **Client-generated `client_id` (UUID)**, unique per sender; a retry returns the original message | Retries after a timeout must not create duplicates | [ADR 0003](../../adr/0003-websocket-protocol.md) |
| K | WebSocket library | **`github.com/coder/websocket`** | Context-aware, small, checks `Origin` by default (stops cross-site WebSocket hijacking) | — |
| L | DB access | **Hand-written SQL with `pgx`** (sqlc stays in 0.2) | Every query is visible in one place while learning indexes and N+1 | — |
| M | Migrations 🔹 | **`golang-migrate`**, SQL files embedded in the binary, applied on startup | Roadmap pick; one binary carries its schema | — |
| N | Frontend stack 🔹 | **React Router, TanStack Query, Tailwind v4** | The "fixed from day 1" stack in [01-app-versions](../../01-app-versions.md) | — |

---

## Section 1 — Back-of-envelope: what do 100 users generate?

Before designing, size the problem (the habit this phase is meant to build).

| Quantity | Assumption | Result |
|---|---|---|
| Concurrent users | all 100 online at peak (pessimistic) | 100 WebSockets |
| Send rate | an active chatter sends 1 msg / 30 s | **≈ 3.3 msg/s** |
| Fan-out | average chat has 5 members | **≈ 17 deliveries/s** |
| Storage per message | ~200 B text + row/index overhead | ≈ 500 B |
| Daily volume | 100 users × 200 msgs/day | 20k msgs/day ≈ **10 MB/day** |
| Memory per WebSocket | 2 goroutines (~8 KB stacks) + buffers | ~50 KB → **5 MB** for 100 |
| bcrypt cost 12 | ~250 ms of one CPU core per login | **100 logins at once ≈ 25 CPU-seconds** |

**Conclusion:** chat traffic is tiny for one server and one Postgres (both handle
thousands of writes/s). The scary number is the last one — a burst of logins is far
more expensive than all the chatting. Prediction for Part 1.4: **the first bottleneck
is bcrypt during the login burst, not messaging.**

---

## Section 2 — Backend layout (modular monolith)

```
backend/
├── cmd/api/main.go            # wires modules together; `api`, `api healthcheck`, `api migrate`
├── migrations/                # 0001_users.up.sql … (embedded with go:embed) 🔹
└── internal/
    ├── config/                # env → typed Config, validated at startup 🔹
    ├── db/                    # pool + migrate-on-start 🔹
    ├── httpx/                 # JSON helpers, error format, middleware (log, recover, auth)
    ├── health/                # (from 0.1, unchanged)
    ├── users/                 # owns `users`: profiles, search, credentials storage
    ├── auth/                  # owns `sessions`: signup, login, refresh, logout, JWTs
    ├── chat/                  # owns `chats`, `chat_members`, `messages`
    ├── realtime/              # WebSocket hub: who is connected, push events to users
    └── testdb/                # integration-test helper: fresh database per test
```

**Module rules** (from [monolith → microservices](../../concepts/monolith-to-microservices.md)):
- Each module owns its tables. `chat` never reads `users` — it asks the `users` module
  through an interface, **in one batch call** (`GetMany(ids)`), which is also the N+1 fix.
- `chat` publishes events through a `Publisher` interface; `realtime` implements it.
  In Phase 3 that interface becomes Redis pub/sub — `chat` won't change.

```
            ┌──────────── HTTP (REST) ────────────┐        ┌── WebSocket ──┐
browser ──► │ auth ──► users ◄── chat ──Publisher──┼──────► │   realtime    │ ──► browsers
            └─────────────────────┬───────────────┘        └───────────────┘
                                  ▼
                               Postgres
```

---

## Section 3 — Part 1.1: Accounts & auth

### Data

```sql
users(id bigint PK, email text UNIQUE, username text UNIQUE, display_name text,
      bio text, password_hash text, created_at, updated_at)
sessions(id uuid PK, user_id → users, token_hash bytea UNIQUE, prev_token_hash bytea,
         user_agent text, created_at, last_used_at, expires_at, revoked_at)
```

- `email` and `username` are stored lower-case, so `UNIQUE` is case-insensitive.
- `users(username text_pattern_ops)` index makes prefix search (`LIKE 'al%'`) use the index.
- Refresh tokens are stored **hashed** (SHA-256): a DB leak doesn't leak live sessions.
  SHA-256, not bcrypt — the token is 256 random bits, so there's nothing to brute-force.

### Endpoints

| Method & path | Auth | Does |
|---|---|---|
| `POST /api/auth/signup` | — | `{email, username, display_name, password}` → `201 {access_token, user}` + refresh cookie |
| `POST /api/auth/login` | — | `{email, password}` → `200 {access_token, user}` + refresh cookie |
| `POST /api/auth/refresh` | cookie | rotates the refresh token → `200 {access_token, user}` + new cookie |
| `POST /api/auth/logout` | cookie | revokes the session, clears the cookie → `204` |
| `GET /api/me` | bearer | my profile |
| `PATCH /api/me` | bearer | `{display_name?, username?, bio?}` |
| `GET /api/users?q=al` | bearer | username prefix search, max 20, excludes me |
| `GET /api/users/{id}` | bearer | public profile |

### Rules

| Field | Rule |
|---|---|
| email | trimmed, lower-cased, has `@`, ≤ 254 chars |
| username | 3–32 chars, `a-z 0-9 _`, lower-cased, unique |
| display name | 1–64 chars after trim |
| bio | ≤ 280 chars |
| password | 8–72 **bytes** (bcrypt silently ignores everything after byte 72) |

- Login with an unknown email still runs bcrypt against a dummy hash, so response time
  doesn't reveal which emails exist (timing attack). Errors say "invalid email or password".
- Access token: HS256 JWT `{sub: user id, sid: session id, iat, exp: +15 min}`, signed
  with `JWT_SECRET` (≥ 32 bytes, required at startup).
- Refresh token: 32 random bytes, base64url. On refresh: if it matches a live session's
  `token_hash` → rotate (old hash moves to `prev_token_hash`). If it matches a
  `prev_token_hash` → **someone replayed a used token** → revoke that session (both the
  thief and the user get logged out; the user just logs in again).
- Refresh and logout also check `Origin` (if present) matches the host — defence in depth
  on top of `SameSite=Strict`.
- Cookie `Secure` flag comes from `COOKIE_SECURE` (off for `http://localhost`, on in Phase 2).

### Frontend

- `/signup`, `/login`, `/profile` pages; everything else needs login (`RequireAuth`).
- On page load the app calls `POST /api/auth/refresh` once: the cookie survives reloads,
  the in-memory access token doesn't.
- The API client attaches `Authorization: Bearer …`; on a `401` it refreshes **once**
  (one shared in-flight refresh, so ten parallel requests cause one refresh) and retries.
- Tabs share the cookie, so two tabs refreshing at the same moment would look like token
  reuse. The client serialises refreshes across tabs with the **Web Locks API**
  (`navigator.locks`) — a real-world gotcha written up in Note 4.

---

## Section 4 — Part 1.2: Chats over REST

### Data

```sql
chats(id bigint PK, kind text CHECK (kind IN ('direct','group')), name text,
      direct_key text UNIQUE,          -- "smallerId:largerId" — one 1:1 chat per pair
      created_by → users, created_at, last_seq bigint DEFAULT 0, last_message_at)
chat_members(chat_id → chats, user_id → users, role text CHECK (role IN ('admin','member')),
             joined_at, last_read_seq bigint DEFAULT 0, PRIMARY KEY (chat_id, user_id))
messages(id bigint PK, chat_id → chats, seq bigint, sender_id → users, body text,
         client_id uuid, created_at, edited_at, deleted_at,
         UNIQUE (chat_id, seq), UNIQUE (sender_id, client_id))
```

| Index | Serves |
|---|---|
| `messages (chat_id, seq)` (the unique constraint) | history pages and catch-up — an index range scan, any depth |
| `chat_members (user_id)` | "my chats" |
| `chats (last_message_at)` sort happens on ≤ a few hundred rows per user → no index needed yet |
| `messages (sender_id, client_id)` (unique) | idempotent retries |

### Sending a message (one transaction)

```
1. SELECT … WHERE sender_id = $me AND client_id = $cid     → exists? return it (200, idempotent)
2. check I'm a member of the chat                          → else 404
3. UPDATE chats SET last_seq = last_seq + 1, last_message_at = now()
     WHERE id = $chat RETURNING last_seq                    → row lock: sends to one chat queue up here
4. INSERT INTO messages (… seq = last_seq …)
5. UPDATE chat_members SET last_read_seq = seq WHERE me     → my own message isn't unread for me
COMMIT → publish message.created to every member → 201
```

The row lock in step 3 is the point: messages in one chat commit **in `seq` order**, so a
client that has seen up to `seq = 41` can safely ask for `after=41` and never miss a
message. (Different chats don't block each other.) If two retries race past step 1, the
unique `(sender_id, client_id)` constraint rejects the second, which then returns the first.

### Endpoints

| Method & path | Does |
|---|---|
| `GET /api/chats` | my chats, newest activity first: name/members, last message preview, `unread` |
| `POST /api/chats/direct` `{user_id}` | find-or-create the 1:1 chat (`200` existing / `201` new) |
| `POST /api/chats/group` `{name, member_ids}` | new group; I'm admin |
| `GET /api/chats/{id}` | one chat with its members |
| `PATCH /api/chats/{id}` `{name}` | rename group (admin) |
| `POST /api/chats/{id}/members` `{user_ids}` | add members (admin) |
| `DELETE /api/chats/{id}/members/{userId}` | remove someone (admin) or leave (self) |
| `PATCH /api/chats/{id}/members/{userId}` `{role}` | promote / demote (admin) |
| `GET /api/chats/{id}/messages?before=&limit=` | history, newest first, `limit ≤ 100` (default 50) |
| `GET /api/chats/{id}/messages?after=&limit=` | catch-up, oldest first |
| `POST /api/chats/{id}/messages` `{body, client_id}` | send |
| `PATCH /api/messages/{id}` `{body}` | edit my message |
| `DELETE /api/messages/{id}` | delete my message (soft: body cleared, `deleted_at` set) |
| `POST /api/chats/{id}/read` `{seq}` | mark read up to `seq` (`GREATEST`, never goes backwards) |

Rules: non-members get **404**, not 403 (don't confirm a chat exists). Group admin rules:
the creator is admin; admins add/remove/promote and rename; anyone can leave; if the last
admin leaves, the longest-standing member becomes admin. Member management on a direct
chat → `400`. Message body: 1–4000 characters after trimming.

### The chat list query (and the N+1 trap)

The naive chat list is 1 query for chats + N queries for each chat's last message + N for
unread counts + N for members' names. Ours is **one query** for chats with last message
(`LEFT JOIN LATERAL … ORDER BY seq DESC LIMIT 1`) and unread (`last_seq − last_read_seq`),
plus **one** batch call to `users.GetMany` for every user id on the page. Two queries, at
any number of chats. Part 1.4 measures it.

### Frontend

- Layout: sidebar (chat list) + main pane (open chat). Routes: `/` (no chat open),
  `/chats/:id`, `/profile`, `/status` (0.1 page kept).
- **New chat** dialog: search users by username → start 1:1. **New group**: name + pick members.
- **Chat view**: messages bottom-up, infinite scroll upward (`useInfiniteQuery` on
  `before=`), composer (Enter sends, Shift+Enter newline), edit/delete on my messages,
  "edited" / "message deleted" markers.
- **Group info** panel: members with roles; admin can add, remove, promote, rename; leave.
- Sending is **optimistic**: the message shows immediately as "sending…", replaced by the
  server's copy (matched by `client_id`); a failure shows "retry".

---

## Section 5 — Part 1.3: Real-time

### Connection

```
1. POST /api/ws/ticket (bearer)  → {ticket}  (HS256, aud "ws", 60 s)
2. GET  /ws?ticket=…  → Upgrade → server checks ticket + Origin → registered in the hub
3. server pings every 25 s; no pong within 10 s → connection closed
4. client: on close → reconnect with exponential backoff + jitter (0.5 s … 30 s),
   on reconnect → catch up
```

### Events (server → client, JSON)

| `type` | Payload | Sent to |
|---|---|---|
| `message.created` | `{chat_id, message}` | all members |
| `message.updated` | `{chat_id, message}` | all members |
| `message.deleted` | `{chat_id, message}` (tombstone) | all members |
| `chat.created` | `{chat_id}` | all members of a new chat |
| `chat.updated` | `{chat_id}` | members — rename, members added/removed, role change |
| `chat.removed` | `{chat_id}` | a user who was removed or left |
| `chat.read` | `{chat_id, seq}` | **my other tabs** (unread badge syncs across tabs) |

Client → server: nothing except pong frames (sending is REST — Decision H).

### The hub

- `map[userID] → set of connections`, guarded by a mutex. A user can have several tabs.
- Each connection has a **buffered outbox** (64 events) and one writer goroutine. Publishing
  never blocks: if a client's outbox is full, it's too slow → **close it** (it will
  reconnect and catch up). One slow phone can't stall delivery to everyone else.
- Publishing happens **after** the transaction commits — never announce a message that
  might still roll back.

### Catch-up (the part that makes it reliable)

On every (re)connect the client: refetches the chat list (fresh previews + unread), and for
each chat whose messages are cached, fetches `?after=<highest seq it has>` until
`has_more` is false. Because `seq` is commit-ordered per chat (Decision E), nothing sent
while offline can be skipped. Duplicates (an event and a catch-up page both carrying
`seq 42`) are dropped by `seq`.

### Proxies

Vite: `'/ws': { target: 'ws://localhost:8080', ws: true }`. Caddy: `handle /ws { reverse_proxy api:8080 }`
(Caddy passes `Upgrade` through by itself). The API server's `WriteTimeout` would kill
long-lived connections, so the WebSocket handler clears its deadline via `http.ResponseController`.

---

## Section 6 — Part 1.4: Measure it

- **k6 script** `loadtest/chat.js`: `setup()` signs up 100 users and creates groups of 5;
  100 VUs each log in, open a WebSocket, send one message every 3 s (≈ 10× the estimate)
  for 2 minutes, and record **delivery latency** (send time → `message.created` received by
  each member) as a custom trend.
- **Thresholds:** 0 failed requests, `delivery p95 < 200 ms`, `http_req_duration p95 < 200 ms`.
- **pprof** on a separate, private address (`PPROF_ADDR`, e.g. `127.0.0.1:6060`, off by
  default) — never on the public port.
- `make loadtest` runs it; results and the fixed bottleneck go into
  [1.4 learnings](1.4-measure/learnings.md).

---

## Section 7 — Testing & done when

### Automated tests

**Go unit** (no DB): password rules & hashing, JWT issue/parse/expiry, refresh-token hashing,
validation, error format, the hub (fan-out, slow-consumer eviction), the event JSON.

**Go integration** (real Postgres, fresh database per test via `internal/testdb`, skipped when
`TEST_DATABASE_URL` is unset):
- auth: signup → login → refresh rotates → old token replay revokes session → logout
- users: search by prefix, profile update, username taken → 409
- chat: direct find-or-create is idempotent; group admin rules; non-member gets 404;
  send assigns consecutive `seq`; idempotent `client_id`; `before`/`after` pagination;
  unread counts; edit/delete only by sender; **20 concurrent sends to one chat → seq 1…20 with no gaps**
- realtime: a WebSocket client receives `message.created` after a REST send; a bad ticket is rejected

**React** (Vitest + Testing Library): API client refresh-once-and-retry, login/signup forms,
chat list rendering + unread, event → cache updates (pure functions), reconnect backoff.

**End to end:** `scripts/e2e.sh` boots the API + web build against a scratch database and
drives two Chromium users with Playwright: sign up both, start a chat, message arrives live.

### Done when

- [ ] `make test` passes (Go unit + integration, React)
- [ ] Two browsers, two users: a message appears in the other browser live (< 200 ms locally)
- [ ] Edit/delete appear live; unread counts right; reconnect catches up after the API restarts
- [ ] `make loadtest`: 100 users, no errors, thresholds met; bottleneck found and fixed, written up
- [ ] ADRs 0001–0003 written; new concepts in `Notes.md`; tracker shows Phase 1 ✅
