-- +goose Up
CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    id_hash TEXT NOT NULL UNIQUE,
    refresh_token_hash TEXT NULL,
    prev_refresh_token_hashes JSONB NULL,
    family_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);

CREATE INDEX sessions_refresh_token_hash ON sessions (refresh_token_hash);
CREATE INDEX sessions_user_id ON sessions (user_id);
CREATE INDEX sessions_family_id ON sessions (family_id);
CREATE INDEX sessions_expires_at ON sessions (expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;
