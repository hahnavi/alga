-- +goose Up

-- teams ---------------------------------------------------------------------
CREATE TABLE teams (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_teams_set_updated_at BEFORE UPDATE ON teams FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- team_members --------------------------------------------------------------
CREATE TABLE team_members (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'lead')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX team_members_team_id_user_id ON team_members (team_id, user_id);
CREATE INDEX team_members_user_id ON team_members (user_id);

-- escalation_policies -------------------------------------------------------
CREATE TABLE escalation_policies (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    repeat_count INT NOT NULL DEFAULT 3 CHECK (repeat_count >= 0),
    levels JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_escalation_policies_set_updated_at BEFORE UPDATE ON escalation_policies FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- on_call_schedules ---------------------------------------------------------
CREATE TABLE on_call_schedules (
    id UUID PRIMARY KEY,
    team_id UUID NULL REFERENCES teams (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX on_call_schedules_team_id ON on_call_schedules (team_id) WHERE team_id IS NOT NULL;
CREATE TRIGGER trg_on_call_schedules_set_updated_at BEFORE UPDATE ON on_call_schedules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- schedule_layers -----------------------------------------------------------
CREATE TABLE schedule_layers (
    id UUID PRIMARY KEY,
    schedule_id UUID NOT NULL REFERENCES on_call_schedules (id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    rotation_type TEXT NOT NULL DEFAULT 'weekly' CHECK (rotation_type IN ('daily', 'weekly', 'custom')),
    rotation_interval INT NOT NULL DEFAULT 1 CHECK (rotation_interval > 0),
    start_date TIMESTAMPTZ NOT NULL DEFAULT now(),
    end_date TIMESTAMPTZ NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    start_time TEXT NOT NULL DEFAULT '00:00',
    end_time TEXT NOT NULL DEFAULT '',
    days_of_week JSONB NOT NULL DEFAULT '[]',
    priority INT NOT NULL DEFAULT 0 CHECK (priority >= 0),
    user_ids JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX schedule_layers_schedule_id ON schedule_layers (schedule_id);
CREATE UNIQUE INDEX schedule_layers_schedule_id_priority ON schedule_layers (schedule_id, priority);
CREATE TRIGGER trg_schedule_layers_set_updated_at BEFORE UPDATE ON schedule_layers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- schedule_overrides --------------------------------------------------------
CREATE TABLE schedule_overrides (
    id UUID PRIMARY KEY,
    schedule_id UUID NOT NULL REFERENCES on_call_schedules (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    created_by UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX schedule_overrides_schedule_id ON schedule_overrides (schedule_id);
CREATE INDEX schedule_overrides_user_id ON schedule_overrides (user_id);
CREATE INDEX schedule_overrides_created_by ON schedule_overrides (created_by);
CREATE INDEX schedule_overrides_start_at_end_at ON schedule_overrides (start_at, end_at);

-- credential_providers ------------------------------------------------------
CREATE TABLE credential_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    type TEXT NOT NULL DEFAULT 'internal' CHECK (type IN ('internal', 'hashicorp_vault', 'aws_secrets_manager', 'gcp_secret_manager', 'azure_key_vault')),
    config_encrypted TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX credential_providers_name ON credential_providers (name);
CREATE INDEX credential_providers_enabled ON credential_providers (enabled);
CREATE TRIGGER trg_credential_providers_set_updated_at BEFORE UPDATE ON credential_providers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- shared_secrets ------------------------------------------------------------
CREATE TABLE shared_secrets (
    id UUID PRIMARY KEY,
    provider_id UUID NOT NULL REFERENCES credential_providers (id) ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (name <> ''),
    secret_id TEXT NOT NULL CHECK (secret_id <> ''),
    description TEXT NOT NULL DEFAULT '',
    remote_ref TEXT NOT NULL DEFAULT '',
    value_encrypted TEXT NOT NULL DEFAULT '',
    value_configured BOOLEAN NOT NULL DEFAULT false,
    allowed_agent_ids JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX shared_secrets_secret_id ON shared_secrets (secret_id);
CREATE INDEX shared_secrets_provider_id ON shared_secrets (provider_id);
CREATE UNIQUE INDEX shared_secrets_provider_id_name ON shared_secrets (provider_id, name);
CREATE TRIGGER trg_shared_secrets_set_updated_at BEFORE UPDATE ON shared_secrets FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS shared_secrets CASCADE;
DROP TABLE IF EXISTS credential_providers CASCADE;
DROP TABLE IF EXISTS schedule_overrides CASCADE;
DROP TABLE IF EXISTS schedule_layers CASCADE;
DROP TABLE IF EXISTS on_call_schedules CASCADE;
DROP TABLE IF EXISTS escalation_policies CASCADE;
DROP TABLE IF EXISTS team_members CASCADE;
DROP TABLE IF EXISTS teams CASCADE;
