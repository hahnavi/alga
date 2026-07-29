-- +goose Up
CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    token_hash TEXT NOT NULL UNIQUE CHECK (token_hash <> ''),
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT password_reset_tokens_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE INDEX password_reset_tokens_user_id ON password_reset_tokens (user_id);
CREATE INDEX password_reset_tokens_expires_at ON password_reset_tokens (expires_at);

-- +goose Down
DROP TABLE IF EXISTS password_reset_tokens;
