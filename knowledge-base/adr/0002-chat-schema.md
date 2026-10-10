# 0002 — Chat schema: per-chat message sequence, counter-based unread, cursor pagination

- **Status:** accepted
- **Date:** 2026-10-10
- **Phase:** 1 (Part 1.2)

## Context
We need: ordered history that scrolls back forever, a chat list with last message and unread
counts, and a reliable way for a reconnecting client to fetch exactly what it missed
("give me everything after X"). 100 users, but the shape should survive growth.

## Options considered
**Ordering / cursor**
1. `created_at` timestamp — ties, clock skew between servers, not unique. ❌
2. Global `bigserial` id — unique and increasing, **but not commit-ordered**: transaction A
   takes id 10, B takes 11, B commits first. A client that saw 11 and asks "after 11"
   never sees 10. ❌ for catch-up.
3. **Per-chat `seq`, assigned by `UPDATE chats SET last_seq = last_seq + 1 … RETURNING`** inside
   the insert transaction. The row lock makes sends to one chat commit in seq order;
   seq is gapless (a rolled-back send doesn't burn a number). Cost: sends to the *same*
   chat are serialised on that row — fine for chat, where one room rarely sees more than a
   few messages per second.

**Unread counts**
1. `COUNT(*) WHERE seq > last_read` per chat — grows with history, N queries or a heavy join.
2. **`chats.last_seq − chat_members.last_read_seq`** — arithmetic on two columns we already
   maintain. Counts deleted messages too (acceptable).

**Pagination**
1. `OFFSET` — gets slower with depth and shifts when new messages arrive (duplicates/skips).
2. **Keyset/cursor on `(chat_id, seq)`** — every page is an index range scan.

## Decision
`messages(id, chat_id, seq, sender_id, body, client_id, created_at, edited_at, deleted_at)`
with `UNIQUE(chat_id, seq)` and `UNIQUE(sender_id, client_id)`; `chats.last_seq` +
`last_message_at`; `chat_members(chat_id, user_id, role, joined_at, last_read_seq)`;
1:1 chats deduplicated by `direct_key = "min:max"` UNIQUE. Ids are `bigint` identity.
Deletes are soft (body cleared, `deleted_at` set) so seq stays gapless and clients get a
tombstone to render. New group members start with `last_read_seq = last_seq`.

The chat list is one query (join + `LEFT JOIN messages ON seq = last_seq`) plus one batch
`users.GetMany` — two queries regardless of how many chats.

## Consequences
- ✅ "After seq N" catch-up can't miss messages; duplicates are detectable by seq.
- ✅ Unread and history cost the same at message 10 and message 10 million.
- ❌ One hot chat serialises its writers on one row. If a single room ever needs hundreds
  of msgs/s (live-stream chat, Phase 6), revisit: batch writes, or allocate seq elsewhere.
- ❌ `bigint` ids are guessable; non-members get 404 for everything, so guessing reveals
  nothing. Phase 7 (sharding) revisits id generation (Snowflake-style).
- ❌ Foreign keys from `chat` tables to `users` cross a module boundary. Fine in one
  database; they go away when a module gets its own database.
- ❌ Edits/deletes aren't in the seq stream, so a reconnecting client can miss an edit to an
  old message (see [ADR 0003](0003-websocket-protocol.md)). A per-chat change log fixes it
  when we need it (Phase 4, delivery guarantees).
