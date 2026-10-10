-- Part 2.1 — reactions and replies. See phases/phase-2/design.md, section 2.

-- A reply points at another message in the same chat (checked by the API).
-- ON DELETE SET NULL is for hard deletes only; normal deletes are soft (deleted_at),
-- so the quote can still say "deleted message".
ALTER TABLE messages ADD COLUMN reply_to_id bigint REFERENCES messages (id) ON DELETE SET NULL;

CREATE TABLE message_reactions (
    message_id bigint      NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    emoji      text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- One of each emoji per person per message. The primary key's index also
    -- serves "reactions for these messages" (message_id is its first column).
    PRIMARY KEY (message_id, user_id, emoji)
);
