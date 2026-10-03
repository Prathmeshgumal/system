-- Throwaway demo table for Part 0.2 (decision L). Dropped by a new migration in Part 1.1.

-- +goose Up
CREATE TABLE notes (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    body       text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notes_created_at_idx ON notes (created_at DESC);

-- +goose Down
DROP TABLE notes;
