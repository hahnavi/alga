-- +goose Up

-- users ---------------------------------------------------------------------
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer' CHECK (role IN ('admin', 'operator', 'viewer')),
    full_name TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    phone_country TEXT NOT NULL DEFAULT '',
    failed_login_attempts INT NOT NULL DEFAULT 0 CHECK (failed_login_attempts >= 0),
    locked_until TIMESTAMPTZ NULL,
    last_failed_login TIMESTAMPTZ NULL,
    last_login_at TIMESTAMPTZ NULL,
    last_login_ip TEXT NOT NULL DEFAULT '',
    google_id TEXT NOT NULL DEFAULT '',
    slack_user_id TEXT NOT NULL DEFAULT '',
    slack_display_name TEXT NOT NULL DEFAULT '',
    notification_preferences JSONB NULL,
    voice_opt_out BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX users_google_id ON users (google_id);
CREATE UNIQUE INDEX users_slack_user_id ON users (slack_user_id) WHERE slack_user_id <> '';
CREATE TRIGGER trg_users_set_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- agent_tokens --------------------------------------------------------------
CREATE TABLE agent_tokens (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    agent_type TEXT NOT NULL DEFAULT 'hermes' CHECK (agent_type IN ('hermes', 'openclaw', 'other')),
    token_hash TEXT NOT NULL UNIQUE,
    lookup_prefix TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    revoked BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    scope TEXT DEFAULT '',
    label_selectors JSONB,
    default_for_investigation BOOLEAN DEFAULT false,
    capabilities JSONB DEFAULT '["investigate"]'
);
CREATE INDEX idx_agent_tokens_lookup_prefix ON agent_tokens (lookup_prefix) WHERE revoked = false AND enabled = true;
CREATE INDEX idx_agent_tokens_expires_at ON agent_tokens (expires_at);
CREATE INDEX idx_agent_tokens_default_for_investigation ON agent_tokens (default_for_investigation) WHERE default_for_investigation = true;
CREATE TRIGGER trg_agent_tokens_set_updated_at BEFORE UPDATE ON agent_tokens FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- sessions ------------------------------------------------------------------
CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    id_hash TEXT NOT NULL UNIQUE,
    refresh_token_hash TEXT NULL,
    prev_refresh_token_hashes JSONB NULL,
    family_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_refresh_token_hash ON sessions (refresh_token_hash);
CREATE INDEX sessions_user_id ON sessions (user_id);
CREATE INDEX sessions_family_id ON sessions (family_id);
CREATE INDEX sessions_expires_at ON sessions (expires_at);

-- personal_access_tokens ----------------------------------------------------
CREATE TABLE personal_access_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (name <> '' AND length(name) <= 128),
    token_hash TEXT NOT NULL UNIQUE CHECK (token_hash <> ''),
    lookup_prefix TEXT NOT NULL CHECK (lookup_prefix <> ''),
    permissions JSONB NOT NULL,
    expires_at TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX personal_access_tokens_lookup_prefix ON personal_access_tokens (lookup_prefix) WHERE revoked = false;
CREATE INDEX personal_access_tokens_user_id ON personal_access_tokens (user_id);
CREATE INDEX personal_access_tokens_expires_at ON personal_access_tokens (expires_at);

-- password_reset_tokens -----------------------------------------------------
CREATE TABLE password_reset_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE CHECK (token_hash <> ''),
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_reset_tokens_user_id ON password_reset_tokens (user_id);
CREATE INDEX password_reset_tokens_expires_at ON password_reset_tokens (expires_at);

-- webhook_tokens ------------------------------------------------------------
CREATE TABLE webhook_tokens (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    token_hash TEXT NOT NULL UNIQUE CHECK (token_hash <> ''),
    lookup_prefix TEXT NOT NULL CHECK (lookup_prefix <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NULL,
    revoked BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX webhook_tokens_lookup_prefix ON webhook_tokens (lookup_prefix) WHERE revoked = false;
CREATE INDEX webhook_tokens_expires_at ON webhook_tokens (expires_at);

-- oidc_providers ------------------------------------------------------------
CREATE TABLE oidc_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    issuer TEXT NOT NULL CHECK (issuer <> ''),
    client_id TEXT NOT NULL CHECK (client_id <> ''),
    client_secret_encrypted TEXT NOT NULL DEFAULT '',
    scopes JSONB NOT NULL DEFAULT '["openid", "email", "profile"]',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX oidc_providers_enabled ON oidc_providers (enabled);
CREATE UNIQUE INDEX oidc_providers_name ON oidc_providers (name);
CREATE UNIQUE INDEX oidc_providers_issuer ON oidc_providers (issuer);
CREATE TRIGGER trg_oidc_providers_set_updated_at BEFORE UPDATE ON oidc_providers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- oidc_identities -----------------------------------------------------------
CREATE TABLE oidc_identities (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES oidc_providers (id) ON DELETE CASCADE,
    subject TEXT NOT NULL CHECK (subject <> ''),
    issuer TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX oidc_identities_provider_id_subject ON oidc_identities (provider_id, subject);
CREATE INDEX oidc_identities_user_id ON oidc_identities (user_id);
CREATE TRIGGER trg_oidc_identities_set_updated_at BEFORE UPDATE ON oidc_identities FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS oidc_identities CASCADE;
DROP TABLE IF EXISTS oidc_providers CASCADE;
DROP TABLE IF EXISTS webhook_tokens CASCADE;
DROP TABLE IF EXISTS password_reset_tokens CASCADE;
DROP TABLE IF EXISTS personal_access_tokens CASCADE;
DROP TABLE IF EXISTS sessions CASCADE;
DROP TABLE IF EXISTS agent_tokens CASCADE;
DROP TABLE IF EXISTS users CASCADE;
