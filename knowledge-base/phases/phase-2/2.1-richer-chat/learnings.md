# Part 2.1 — Richer chat: Learnings

Spec: [design.md § 2](../design.md#section-2--part-21-richer-chat) · Concept: [Note 8](../../../Notes.md#8-temporary-vs-stored-events-and-write-amplification)

## What was built

| Piece | Where | Read this first |
|---|---|---|
| Reactions + replies schema | `backend/migrations/0003_reactions_replies.up.sql` | the primary key that doubles as the lookup index |
| Store: reactions, quotes in batches | `backend/internal/chat/store.go` | `ReactionsFor`, `MessagesByIDs` |
| Views: 3 extra queries per page, not per message | `backend/internal/chat/service.go` | `views` |
| Typing relay + read receipts to everyone | `service.go` `Typing`, `MarkRead` | what's stored and what isn't |
| Client frames over the socket, rate-limited | `backend/internal/realtime/handler.go`, `bucket.go` | `read`, `allow` |
| UI: reactions, replies, ✓/✓✓, "Seen by", typing line | `web/src/chat/MessageList.tsx`, `ChatView.tsx`, `receipts.ts`, `realtime/typing.ts` | |
| Notifications + `(3) Relay` title, dark mode | `web/src/lib/notify.ts`, `theme.ts`, `public/theme-init.js` | |

Tests: `internal/chat` (idempotent reactions, replies), `internal/app/richchat_test.go`
(reactions/replies over HTTP, receipts to every member, typing relay, membership, rate limit),
`internal/realtime/bucket_test.go`, React unit tests (receipts, typing expiry, theme, message
list), e2e `richchat.spec.ts` in two browsers.

## Already observed while building

- **A rate limit that was too clever.** The first version dropped any typing frame within 500 ms
  of the previous one. Fast in tests, broken in real use: send a message, start typing the next
  one at once — the frame arrived 284 ms after the previous one and was dropped, so the other
  person never saw "typing…". Found by logging frames on both sides of a scripted conversation;
  the e2e test had only checked the *first* typing event. Now a token bucket (burst 5, 2/s),
  plus a regression test for exactly that sequence.
- **One page of history = 4 queries**, whatever its size: messages, reactions, quoted messages,
  profiles (for senders *and* quoted senders together).
- **Read receipts are the noisiest feature.** In the 100-user soak test there were more read
  calls than messages, and ~4 WebSocket events per delivered message (Note 8).
- **Debugging trap:** an old API process still held port 8080, so the "debug build" never ran.
  `lsof -iTCP:8080 -sTCP:LISTEN` before trusting what's answering.
- Moving the dark-mode script out of `index.html` into `public/theme-init.js` costs one tiny
  request but lets production forbid inline scripts (CSP, Part 2.3).

## Experiments to run

### 1. Watch the amplification
Open a group of 5 in 5 browsers (or private windows). DevTools → Network → WS on one of them.
Send one message. Count the frames that arrive (message, then a `chat.read` from each reader).
Now imagine 200 members.

**Your results:**

### 2. Lose a typing event on purpose
In `realtime/hub.go`, make `Publish` skip every second `typing` event. Does anyone notice? Do
the same for `message.created` — what breaks, and what repairs it (catch-up)?

**Your results:**

### 3. Notifications
Turn notifications on, switch to another tab, get a message from a second browser. Then
`(1) Relay` in the tab title. What happens with 3 messages from the same chat (the `tag`)?

**Your results:**
