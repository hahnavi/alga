-- +goose Up
CREATE TABLE ldap_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE CHECK (name <> ''),
    url TEXT NOT NULL,
    base_dn TEXT NOT NULL,
    bind_dn TEXT NOT NULL,
    bind_password_encrypted TEXT NOT NULL,
    login_attribute TEXT NOT NULL,
    id_attribute TEXT NOT NULL,
    email_attribute TEXT NOT NULL,
    start_tls BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_ldap_providers_set_updated_at BEFORE UPDATE ON ldap_providers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE ldap_identities (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES ldap_providers (id) ON DELETE CASCADE,
    subject TEXT NOT NULL CHECK (subject <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider_id, subject),
    UNIQUE (provider_id, user_id)
);
CREATE INDEX ldap_identities_user_id ON ldap_identities (user_id);

-- +goose Down
DROP TABLE ldap_identities;
DROP TABLE ldap_providers;
