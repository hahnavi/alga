-- +goose Up
CREATE TABLE personal_access_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 128),
    token_hash TEXT NOT NULL UNIQUE CHECK (token_hash <> ''),
    lookup_prefix TEXT NOT NULL CHECK (lookup_prefix <> ''),
    permissions JSONB NOT NULL,
    expires_at TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked BOOLEAN NOT NULL DEFAULT false,
    CONSTRAINT personal_access_tokens_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE INDEX personal_access_tokens_lookup_prefix ON personal_access_tokens (lookup_prefix) WHERE revoked = false;
CREATE INDEX personal_access_tokens_user_id ON personal_access_tokens (user_id);
CREATE INDEX personal_access_tokens_expires_at ON personal_access_tokens (expires_at);

-- +goose Down
DROP TABLE IF EXISTS personal_access_tokens;
