-- +goose Up
CREATE TABLE oidc_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    issuer TEXT NOT NULL CHECK (issuer <> ''),
    client_id TEXT NOT NULL CHECK (client_id <> ''),
    client_secret_encrypted TEXT NOT NULL DEFAULT '',
    scopes JSONB NOT NULL DEFAULT '["openid", "email", "profile"]',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX oidc_providers_enabled ON oidc_providers (enabled);
CREATE UNIQUE INDEX oidc_providers_name ON oidc_providers (name);
CREATE UNIQUE INDEX oidc_providers_issuer ON oidc_providers (issuer);

CREATE TABLE oidc_identities (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    provider_id UUID NOT NULL,
    subject TEXT NOT NULL CHECK (subject <> ''),
    issuer TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT oidc_identities_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT oidc_identities_provider_id_fk FOREIGN KEY (provider_id) REFERENCES oidc_providers (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX oidc_identities_provider_id_subject ON oidc_identities (provider_id, subject);
CREATE INDEX oidc_identities_user_id ON oidc_identities (user_id);

-- +goose Down
DROP TABLE IF EXISTS oidc_identities;
DROP TABLE IF EXISTS oidc_providers;
