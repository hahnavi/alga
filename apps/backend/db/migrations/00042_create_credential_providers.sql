-- +goose Up
CREATE TABLE credential_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    type TEXT NOT NULL DEFAULT 'internal' CHECK (type IN ('internal', 'hashicorp_vault', 'aws_secrets_manager', 'gcp_secret_manager', 'azure_key_vault')),
    config_encrypted TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX credential_providers_name ON credential_providers (name);
CREATE INDEX credential_providers_enabled ON credential_providers (enabled);

-- +goose Down
DROP TABLE IF EXISTS credential_providers;
