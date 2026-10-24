-- +goose Up

-- saml_providers ------------------------------------------------------------
CREATE TABLE saml_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    idp_entity_id TEXT NOT NULL DEFAULT '',
    metadata_xml TEXT NOT NULL DEFAULT '',
    email_attribute TEXT NOT NULL DEFAULT 'email' CHECK (email_attribute <> ''),
    enabled BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX saml_providers_name ON saml_providers (name);
CREATE UNIQUE INDEX saml_providers_entity_id ON saml_providers (idp_entity_id) WHERE idp_entity_id <> '';
CREATE INDEX saml_providers_enabled ON saml_providers (enabled);
CREATE TRIGGER trg_saml_providers_set_updated_at BEFORE UPDATE ON saml_providers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- saml_identities -----------------------------------------------------------
CREATE TABLE saml_identities (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES saml_providers (id) ON DELETE CASCADE,
    subject TEXT NOT NULL CHECK (subject <> ''),
    email TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider_id, subject)
);
CREATE INDEX saml_identities_user_id ON saml_identities (user_id);
CREATE TRIGGER trg_saml_identities_set_updated_at BEFORE UPDATE ON saml_identities FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS saml_identities CASCADE;
DROP TABLE IF EXISTS saml_providers CASCADE;
