# Phase 1 — Chat MVP (v0.1)

**Goal:** you and a couple of friends on your local network can chat in real time — accounts,
1:1 and group chats, live messages, edits and deletes, unread counts, history, reconnects —
and we know what one server can take.

**Docs:** [design](design.md) · [build plan](plan.md) · ADRs [0001 auth](../../adr/0001-auth-tokens.md),
[0002 schema](../../adr/0002-chat-schema.md), [0003 WebSocket protocol](../../adr/0003-websocket-protocol.md) ·
Notes [4](../../Notes.md#4-sessions-jwts--where-tokens-live), [5](../../Notes.md#5-http-vs-polling-vs-websockets),
[6](../../Notes.md#6-ordering--cursor-pagination), [7](../../Notes.md#7-password-hashing--cpu-heavy-work)

| Part | Build | Learn | Status | Docs |
|---|---|---|---|---|
| 1.1 Accounts & auth | Signup, login, logout, rotating refresh tokens, profile, auth pages | Password hashing, JWT vs sessions, cookies, XSS/CSRF | ✅ built — experiments are yours | [learnings](1.1-accounts-auth/learnings.md) |
| 1.2 Chats over REST | 1:1 + group chats, members/admins, send, history, chat list | Data modelling, indexes, cursor pagination, N+1, estimating load | ✅ built — experiments are yours | [learnings](1.2-chats-rest/learnings.md) |
| 1.3 Real-time | WebSocket hub, live messages/edits/deletes, unread counts, reconnect + catch-up | WebSocket protocol, goroutine per connection, heartbeats, ordering, idempotency | ✅ built — experiments are yours | [learnings](1.3-realtime/learnings.md) |
| 1.4 Measure it | k6 test with 100 users, pprof, fixed the first bottleneck (bcrypt burst) | Latency percentiles, profiling, bulkheads | ✅ measured + fixed | [learnings](1.4-measure/learnings.md) |

Phase 1 also built the parts of 0.2 / 0.3 it needed (config, migrations, test-database helper,
router, TanStack Query, Tailwind, Playwright) — see [design.md](design.md#about-phase-0-parts-02-and-03).

**Ends with:** v0.1 — see [01-app-versions.md](../../01-app-versions.md#v01--it-works-on-my-machine-end-of-phase-1).
