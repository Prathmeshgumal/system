-- Part 1.1 — accounts and login sessions. See phases/phase-1/design.md, section 3.

CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         text        NOT NULL UNIQUE,  -- stored lower-case → unique ignores case
    username      text        NOT NULL UNIQUE,  -- stored lower-case
    display_name  text        NOT NULL,
    bio           text        NOT NULL DEFAULT '',
    password_hash text        NOT NULL,         -- bcrypt, includes its own salt and cost
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- The UNIQUE index on username can't serve LIKE 'al%' under most collations;
-- text_pattern_ops compares byte by byte, which makes prefix search an index range scan.
CREATE INDEX users_username_prefix_idx ON users (username text_pattern_ops);

-- One row per login (per device/browser). The refresh token itself is never stored,
-- only its SHA-256, so a database leak doesn't hand out live sessions.
CREATE TABLE sessions (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash      bytea       NOT NULL UNIQUE,  -- current refresh token
    prev_token_hash bytea,                        -- the one it replaced: seeing it again = replay
    user_agent      text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_used_at    timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    revoked_at      timestamptz
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_prev_token_hash_idx ON sessions (prev_token_hash);
