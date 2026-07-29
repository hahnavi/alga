-- +goose Up
CREATE TABLE shared_secrets (
    id UUID PRIMARY KEY,
    provider_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (name <> ''),
    secret_id TEXT NOT NULL CHECK (secret_id <> ''),
    description TEXT NOT NULL DEFAULT '',
    remote_ref TEXT NOT NULL DEFAULT '',
    value_encrypted TEXT NOT NULL DEFAULT '',
    value_configured BOOLEAN NOT NULL DEFAULT false,
    allowed_agent_ids JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT shared_secrets_provider_id_fk FOREIGN KEY (provider_id) REFERENCES credential_providers (id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX shared_secrets_secret_id ON shared_secrets (secret_id);
CREATE INDEX shared_secrets_provider_id ON shared_secrets (provider_id);
CREATE UNIQUE INDEX shared_secrets_provider_id_name ON shared_secrets (provider_id, name);

-- +goose Down
DROP TABLE IF EXISTS shared_secrets;
