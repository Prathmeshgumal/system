# 0003 — WebSocket protocol: push-only events, REST for writes, ticket auth, catch-up by seq

- **Status:** accepted
- **Date:** 2026-10-10
- **Phase:** 1 (Part 1.3)

## Context
Messages, edits and deletes must appear for everyone within ~200 ms; a dropped connection
must heal itself without losing anything. One API server for now; several in Phase 3.
See [Note 5](../Notes.md#5-http-vs-polling-vs-websockets) for why WebSockets at all.

## Options considered
1. **Everything over the socket** (send, edit, read markers, plus events) — one channel,
   but then we re-invent request ids, acks, error codes, validation and auth per frame, and
   lose HTTP tooling (curl, logs, status codes, k6 checks).
2. **Push-only socket + REST for every write** — the socket only carries server → client
   events; sending is `POST /api/chats/{id}/messages`. Slack's model for years.
3. Server-Sent Events instead of WebSockets — simpler, but one-directional forever, and
   Phase 2 (typing indicators) and Phase 6 (call signalling) want client → server frames.

## Decision
Option 2, over WebSockets (`github.com/coder/websocket`).

- **Auth:** `POST /api/ws/ticket` (bearer) → 60 s ticket; `GET /ws?ticket=…`. Browsers can't
  set headers on a WebSocket, and a 15-minute access token in a URL ends up in logs.
- **Origin** is checked on upgrade (cross-site WebSocket hijacking).
- **Events:** `message.created|updated|deleted {chat_id, message}`, `chat.created|updated|removed
  {chat_id}`, `chat.read {chat_id, seq}` (to the reader's other tabs).
- **Server side:** a hub maps user → connections (all tabs). Each connection has a 64-event
  outbox and one writer goroutine; a full outbox closes the connection (slow consumer) so
  publishing never blocks. Events are published only after the DB transaction commits.
  Ping every 25 s, 10 s to answer.
- **Idempotent sends:** client-made `client_id` (UUID); a retry returns the original (200).
  The optimistic bubble is replaced when the message with the same `client_id` arrives.
- **Reconnect:** exponential backoff 0.5 s → 30 s with jitter. On **every** open (including the
  first — messages sent between page load and socket open would otherwise be lost), refetch
  the chat list and fetch `?after=<newest seq held>` for each loaded chat until `has_more`
  is false.

## Consequences
- ✅ One write path with validation, status codes and tests; the socket stays tiny.
- ✅ Catch-up is exact for new messages because seq is commit-ordered ([ADR 0002](0002-chat-schema.md)).
- ❌ Two connections per client (HTTP + WS) — irrelevant at this scale.
- ❌ The hub is in-process: users on different servers can't reach each other. That's
  Phase 3's "WebSocket problem", solved with Redis pub/sub behind the same `Publisher` interface.
- ❌ Edits/deletes to older messages during an outage aren't caught up (history is marked
  stale and refetched when a chat is reopened). Revisit with a change log in Phase 4.
- ❌ Browsers can't see a silently dead connection (no FIN) until TCP gives up; only the
  server pings. Client-side heartbeats are a Phase 2/3 improvement.
