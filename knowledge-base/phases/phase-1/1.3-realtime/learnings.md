# Part 1.3 — Real-time: Learnings

Spec: [design.md § 5](../design.md#section-5--part-13-real-time) · Decision: [ADR 0003](../../../adr/0003-websocket-protocol.md) ·
Concept: [Note 5](../../../Notes.md#5-http-vs-polling-vs-websockets)

## What was built

| Piece | Where | Read this first |
|---|---|---|
| Hub: who's connected, fan-out | `backend/internal/realtime/hub.go` | `Publish` — why it never blocks |
| `/ws` endpoint + ticket | `backend/internal/realtime/handler.go` | the `select` loop |
| Proxies | `web/vite.config.ts`, `web/Caddyfile` | `ws: true`, `handle /ws` |
| Client socket with backoff | `web/src/realtime/socket.ts` | `backoffDelay` |
| Events → cache, catch-up | `web/src/realtime/events.ts` | `catchUp` |
| Optimistic sending | `web/src/chat/pending.ts`, `queries.ts` `useSend` | matching by `client_id` |
| Browser tests | `web/e2e/chat.spec.ts` | the reconnect test |

Tests: `internal/realtime/hub_test.go` (fan-out to every tab, slow-client eviction),
`internal/app/realtime_test.go` (real WebSocket client: events after REST calls, forged
ticket, foreign Origin, connection outlives `WriteTimeout`), `web/src/realtime/socket.test.ts`,
and 4 Playwright tests with two real browsers.

## Already observed while building

- **Delivery between two browsers: ~70–100 ms** in the e2e test, measured from pressing Enter
  in one browser to the text being visible in the other — and that includes Playwright's own
  polling. The server-side part is ~4 ms (see 1.4).
- **We assumed the server's 15 s `WriteTimeout` would kill WebSockets** and wrote code to clear
  deadlines. A test showed it was never needed: `net/http` clears connection deadlines on
  `Hijack` (`server.go`, `hijackLocked`). The test stays as a guard; the code went.
- **Playwright's `setOffline(true)` doesn't close a WebSocket to localhost**, so our first
  "Wi-Fi drops" test never dropped anything. The test now cuts the socket with
  `page.routeWebSocket` and refuses reconnects during the "outage".
- Duplicate `POST /read` calls: the mark-read effect re-ran before the optimistic cache update
  landed. Fixed with a ref holding the last requested seq.
- Requests the browser aborts (switching chats mid-fetch) were logged as `ERROR … context
  canceled` 500s. Now they're logged as `499` and not as errors.

## Experiments to run

### 1. Watch the frames
DevTools → Network → WS → `/ws` → Messages. Send, edit, delete from another browser. Which
events arrive? Mark a chat read in tab A — what does tab B receive?

**Your results:**

### 2. Kill the server mid-chat
With two browsers open, stop the API (run `make db`, `make web`, `make api` in separate
terminals and Ctrl+C the API). What does the sidebar status show, and how far apart are the
reconnect attempts (Network tab)? Try sending while it's down — what does the bubble show?
Start the API again: does each browser reconnect by itself, and does "retry" deliver the message once?

**Your results:**

### 3. A slow consumer
In `hub.go` set `outboxSize = 2`, and in `handler.go` add `time.Sleep(time.Second)` before
`write(...)`. Send 5 quick messages. What happens to the slow tab, and what does it do next?
(Put both back afterwards.)

**Your results:**

### 4. Known gap: edits during an outage
Open a chat in browser B, cut B's network (DevTools → Network → Offline — on a non-localhost
URL), edit an *older* message from A, reconnect B. Is the edit shown? Reopen the chat. Why?
(ADR 0003, Consequences.)

**Your results:**
