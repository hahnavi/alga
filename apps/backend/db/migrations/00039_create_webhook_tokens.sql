-- +goose Up
CREATE TABLE webhook_tokens (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    token_hash TEXT NOT NULL UNIQUE CHECK (token_hash <> ''),
    lookup_prefix TEXT NOT NULL CHECK (lookup_prefix <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NULL,
    revoked BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX webhook_tokens_lookup_prefix ON webhook_tokens (lookup_prefix) WHERE revoked = false;
CREATE INDEX webhook_tokens_expires_at ON webhook_tokens (expires_at);

-- +goose Down
DROP TABLE IF EXISTS webhook_tokens;
