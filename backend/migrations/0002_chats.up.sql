-- Part 1.2 — chats, members and messages. See phases/phase-1/design.md, section 4,
-- and ADR 0002 for why messages are ordered by a per-chat seq.

CREATE TABLE chats (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind            text        NOT NULL CHECK (kind IN ('direct', 'group')),
    name            text        NOT NULL DEFAULT '',  -- groups only
    direct_key      text        UNIQUE,               -- "smallerId:largerId": one 1:1 chat per pair
    created_by      bigint      NOT NULL REFERENCES users (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seq        bigint      NOT NULL DEFAULT 0,   -- seq of the newest message; bumped under a row lock
    last_message_at timestamptz,                      -- NULL until the first message
    CHECK ((kind = 'direct') = (direct_key IS NOT NULL))
);

CREATE TABLE chat_members (
    chat_id       bigint      NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    user_id       bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role          text        NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    joined_at     timestamptz NOT NULL DEFAULT now(),
    last_read_seq bigint      NOT NULL DEFAULT 0,     -- unread = chats.last_seq - last_read_seq
    PRIMARY KEY (chat_id, user_id)                    -- also serves "members of chat X"
);

-- "Which chats am I in?" — the chat list starts here.
CREATE INDEX chat_members_user_id_idx ON chat_members (user_id);

CREATE TABLE messages (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    chat_id    bigint      NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    seq        bigint      NOT NULL,                  -- 1, 2, 3… within the chat, no gaps
    sender_id  bigint      NOT NULL REFERENCES users (id),
    body       text        NOT NULL,                  -- '' once deleted
    client_id  uuid        NOT NULL,                  -- made by the sender's app: retries reuse it
    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at  timestamptz,
    deleted_at timestamptz,
    UNIQUE (chat_id, seq),        -- history pages and catch-up are range scans on this index
    UNIQUE (sender_id, client_id) -- a retried send can't create a duplicate
);
