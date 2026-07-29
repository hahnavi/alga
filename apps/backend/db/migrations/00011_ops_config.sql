-- +goose Up

-- integrations --------------------------------------------------------------
-- Single-row configuration table (pinned to the fixed singleton UUID by the
-- store). The seven secret-bearing columns store AEAD ciphertext produced by
-- crypto.Keyring (store/integrations.go encryptSecrets); the `_encrypted`
-- suffix documents that at-rest contract, matching oidc_providers /
-- credential_providers. Non-secret columns (ids, urls, telnyx_tts_api_key_ref)
-- remain plaintext.
CREATE TABLE integrations (
    id UUID PRIMARY KEY,
    mattermost_url TEXT NOT NULL DEFAULT '',
    mattermost_webhook_secret_encrypted TEXT NOT NULL DEFAULT '',
    mattermost_team TEXT NOT NULL DEFAULT '',
    mattermost_default_channel TEXT NOT NULL DEFAULT '',
    mattermost_disabled BOOLEAN NOT NULL DEFAULT false,
    slack_bot_token_encrypted TEXT NOT NULL DEFAULT '',
    slack_signing_secret_encrypted TEXT NOT NULL DEFAULT '',
    slack_default_channel TEXT NOT NULL DEFAULT '',
    slack_disabled BOOLEAN NOT NULL DEFAULT false,
    slack_client_id TEXT NOT NULL DEFAULT '',
    slack_client_secret_encrypted TEXT NOT NULL DEFAULT '',
    slack_workspace_name TEXT NOT NULL DEFAULT '',
    slack_workspace_id TEXT NOT NULL DEFAULT '',
    twilio_account_sid TEXT NOT NULL DEFAULT '',
    twilio_auth_token_encrypted TEXT NOT NULL DEFAULT '',
    twilio_from_number TEXT NOT NULL DEFAULT '',
    twilio_disabled BOOLEAN NOT NULL DEFAULT false,
    telnyx_api_key_encrypted TEXT NOT NULL DEFAULT '',
    telnyx_connection_id TEXT NOT NULL DEFAULT '',
    telnyx_from_number TEXT NOT NULL DEFAULT '',
    telnyx_public_key TEXT NOT NULL DEFAULT '',
    telnyx_disabled BOOLEAN NOT NULL DEFAULT false,
    telnyx_tts_voice TEXT NOT NULL DEFAULT '',
    telnyx_tts_language TEXT NOT NULL DEFAULT '',
    telnyx_tts_api_key_ref TEXT NOT NULL DEFAULT '',
    voice_provider TEXT NOT NULL DEFAULT 'twilio' CHECK (voice_provider IN ('twilio', 'telnyx')),
    hermes_platform_url TEXT NOT NULL DEFAULT '',
    hermes_platform_token_encrypted TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT integrations_singleton CHECK (id = '00000000-0000-0000-0000-000000000001')
);
CREATE TRIGGER trg_integrations_set_updated_at BEFORE UPDATE ON integrations FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- system_config -------------------------------------------------------------
-- Single-row JSONB config bag. created_at added; updated_at keeps its DEFAULT
-- for the INSERT/upsert path (the trigger only fires BEFORE UPDATE).
CREATE TABLE system_config (
    id UUID PRIMARY KEY,
    config JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT system_config_singleton CHECK (id = '00000000-0000-0000-0000-000000000001')
);
CREATE TRIGGER trg_system_config_set_updated_at BEFORE UPDATE ON system_config FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- route_rules ---------------------------------------------------------------
CREATE TABLE route_rules (
    id UUID PRIMARY KEY,
    routes JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT route_rules_singleton CHECK (id = '00000000-0000-0000-0000-000000000001')
);
CREATE TRIGGER trg_route_rules_set_updated_at BEFORE UPDATE ON route_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- playbooks -----------------------------------------------------------------
CREATE TABLE playbooks (
    id UUID PRIMARY KEY,
    title TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('procedure', 'mitigation')),
    summary TEXT NULL,
    service_id UUID NULL,
    label_selectors JSONB NULL,
    tags JSONB NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT playbooks_service_id_fk FOREIGN KEY (service_id) REFERENCES services (id) ON DELETE SET NULL,
    CONSTRAINT playbooks_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE CASCADE
);
CREATE TRIGGER trg_playbooks_set_updated_at BEFORE UPDATE ON playbooks FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- playbook_steps ------------------------------------------------------------
CREATE TABLE playbook_steps (
    id UUID PRIMARY KEY,
    playbook_id UUID NOT NULL,
    step_number INT NOT NULL CHECK (step_number > 0),
    title TEXT NOT NULL,
    description TEXT NULL,
    expected_duration TEXT NULL,
    command TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT playbook_steps_playbook_id_fk FOREIGN KEY (playbook_id) REFERENCES playbooks (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX playbook_steps_playbook_id_step_number ON playbook_steps (playbook_id, step_number);
CREATE TRIGGER trg_playbook_steps_set_updated_at BEFORE UPDATE ON playbook_steps FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- maintenance_windows -------------------------------------------------------
-- created_by stores an actor email (api/maintenance.go sets user.Email), not a
-- user UUID, so it intentionally stays TEXT (no FK).
CREATE TABLE maintenance_windows (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    label_matchers JSONB NULL,
    created_by TEXT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT maintenance_windows_time_order CHECK (end_time > start_time)
);
CREATE INDEX maintenance_windows_enabled_start_time_end_time ON maintenance_windows (enabled, start_time, end_time);
CREATE TRIGGER trg_maintenance_windows_set_updated_at BEFORE UPDATE ON maintenance_windows FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- status_pages --------------------------------------------------------------
CREATE TABLE status_pages (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    slug TEXT NOT NULL UNIQUE CHECK (slug <> ''),
    description TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'internal' CHECK (visibility IN ('internal', 'public')),
    enabled BOOLEAN NOT NULL DEFAULT true,
    owner_team_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT status_pages_owner_team_id_fk FOREIGN KEY (owner_team_id) REFERENCES teams (id) ON DELETE SET NULL
);
CREATE INDEX status_pages_enabled ON status_pages (enabled);
CREATE INDEX status_pages_owner_team_id ON status_pages (owner_team_id);
CREATE TRIGGER trg_status_pages_set_updated_at BEFORE UPDATE ON status_pages FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- status_page_components ----------------------------------------------------
CREATE TABLE status_page_components (
    id UUID PRIMARY KEY,
    status_page_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (name <> ''),
    description TEXT NOT NULL DEFAULT '',
    service_id UUID NULL,
    display_order INT NOT NULL DEFAULT 0 CHECK (display_order >= 0),
    status TEXT NOT NULL DEFAULT 'operational' CHECK (status IN ('operational', 'degraded', 'partial_outage', 'major_outage', 'maintenance')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT status_page_components_status_page_id_fk FOREIGN KEY (status_page_id) REFERENCES status_pages (id) ON DELETE CASCADE,
    CONSTRAINT status_page_components_service_id_fk FOREIGN KEY (service_id) REFERENCES services (id) ON DELETE SET NULL
);
CREATE INDEX status_page_components_status_page_id_display_order ON status_page_components (status_page_id, display_order);
CREATE INDEX status_page_components_service_id ON status_page_components (service_id);
CREATE TRIGGER trg_status_page_components_set_updated_at BEFORE UPDATE ON status_page_components FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- heartbeats ----------------------------------------------------------------
-- ping_token_hash + lookup_prefix follow the HMAC-hash + prefix token pattern.
-- created_by stores an actor email (api/heartbeat.go sets user.Email), not a
-- user UUID, so it intentionally stays TEXT (no FK).
CREATE TABLE heartbeats (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    description TEXT NOT NULL DEFAULT '',
    interval_seconds INT NOT NULL CHECK (interval_seconds > 0),
    grace_seconds INT NOT NULL DEFAULT 60 CHECK (grace_seconds >= 0),
    enabled BOOLEAN NOT NULL DEFAULT true,
    owner_team_id UUID NULL,
    status TEXT NOT NULL DEFAULT 'healthy' CHECK (status IN ('healthy', 'expired')),
    severity TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('critical', 'warning', 'info')),
    labels JSONB NULL,
    ping_token_hash TEXT NOT NULL UNIQUE CHECK (ping_token_hash <> ''),
    lookup_prefix TEXT NOT NULL CHECK (lookup_prefix <> ''),
    last_ping_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NULL,
    last_breach_at TIMESTAMPTZ NULL,
    created_by TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT heartbeats_owner_team_id_fk FOREIGN KEY (owner_team_id) REFERENCES teams (id) ON DELETE SET NULL
);
CREATE INDEX heartbeats_enabled_status_expires_at ON heartbeats (enabled, status, expires_at);
CREATE INDEX heartbeats_enabled_last_ping_at ON heartbeats (enabled, last_ping_at);
CREATE INDEX heartbeats_owner_team_id ON heartbeats (owner_team_id);
CREATE INDEX heartbeats_lookup_prefix ON heartbeats (lookup_prefix);
CREATE TRIGGER trg_heartbeats_set_updated_at BEFORE UPDATE ON heartbeats FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS heartbeats CASCADE;
DROP TABLE IF EXISTS status_page_components CASCADE;
DROP TABLE IF EXISTS status_pages CASCADE;
DROP TABLE IF EXISTS maintenance_windows CASCADE;
DROP TABLE IF EXISTS playbook_steps CASCADE;
DROP TABLE IF EXISTS playbooks CASCADE;
DROP TABLE IF EXISTS route_rules CASCADE;
DROP TABLE IF EXISTS system_config CASCADE;
DROP TABLE IF EXISTS integrations CASCADE;
