# Part 1.2 — Chats over REST: Learnings

Spec: [design.md § 4](../design.md#section-4--part-12-chats-over-rest) · Decision: [ADR 0002](../../../adr/0002-chat-schema.md) ·
Concept: [Note 6](../../../Notes.md#6-ordering--cursor-pagination)

## What was built

| Piece | Where | Read this first |
|---|---|---|
| Schema | `backend/migrations/0002_chats.up.sql` | the comments on `last_seq`, `seq`, `client_id` |
| SQL | `backend/internal/chat/store.go` | `Send` (row lock), `History` (cursors), `ListForUser` (one query) |
| Rules: membership, admins, validation, events | `backend/internal/chat/service.go` | `List` (the N+1 fix), `Send` (publish after commit) |
| HTTP | `backend/internal/chat/handler.go` | `toHTTP` — non-members get 404 |
| Chat list, chat view, dialogs, group info | `web/src/chat/` | `queries.ts`, `cache.ts` |

Tests: `internal/chat/store_test.go` (20 concurrent sends → gapless seq, concurrent direct-chat
creation, idempotent retries, cursors, unread), `internal/app/chat_test.go` (permissions,
admin rules, validation), `web/src/chat/cache.test.ts`, `ChatList.test.tsx`.

## Already observed while building

- **Concurrency tests pass on the first try against real Postgres** — 20 parallel sends
  produce seq 1…20, 10 parallel "start chat with Bob" produce one chat, 10 parallel retries
  of one `client_id` produce one message. The unique constraints + row lock do the work.
- Chat list: **2 queries** per request whatever the number of chats (one for chats with last
  message and unread, one `GetMany` for names).
- In the load test (1.4) a send took a median **3.3 ms** end to end, with the transaction
  doing 4 statements.

## Experiments to run

### 1. OFFSET vs cursor
Make one chat big, then compare plans:
```sql
-- make psql
INSERT INTO messages (chat_id, seq, sender_id, body, client_id)
SELECT 1, g, (SELECT created_by FROM chats WHERE id = 1), 'msg ' || g, gen_random_uuid()
FROM generate_series((SELECT last_seq FROM chats WHERE id = 1) + 1, 200000) g;
UPDATE chats SET last_seq = 200000 WHERE id = 1;

EXPLAIN ANALYZE SELECT * FROM messages WHERE chat_id = 1 ORDER BY seq DESC LIMIT 50 OFFSET 150000;
EXPLAIN ANALYZE SELECT * FROM messages WHERE chat_id = 1 AND seq < 50000 ORDER BY seq DESC LIMIT 50;
```
Times? Which index does each use, and how many rows does each read?

**Your results:**

### 2. See the row lock
Two `make psql` terminals:
```sql
-- terminal 1                                   -- terminal 2
BEGIN;
UPDATE chats SET last_seq = last_seq + 1
  WHERE id = 1 RETURNING last_seq;
                                                UPDATE chats SET last_seq = last_seq + 1
                                                  WHERE id = 1 RETURNING last_seq;  -- waits…
COMMIT;                                         -- …now returns the next number
```
Now try it with two *different* chat ids. Why is that the whole point?

**Your results:**

### 3. Count the queries
Set `log_statement = 'all'` (`ALTER SYSTEM SET log_statement = 'all'; SELECT pg_reload_conf();`),
open the chat list, and count statements in `docker compose logs postgres`. Then imagine the
naive version: one query per chat for its last message and one per sender name.

**Your results:**
