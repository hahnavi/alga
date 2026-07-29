-- +goose Up
CREATE TABLE incidents (
    id UUID PRIMARY KEY,
    incident_number BIGINT NOT NULL UNIQUE CHECK (incident_number >= 0),
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'detected' CHECK (status IN ('detected', 'triaging', 'active', 'mitigated', 'resolved', 'closed', 'cancelled')),
    severity TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('critical', 'high', 'warning', 'info')),
    impact_level TEXT NOT NULL DEFAULT 'medium' CHECK (impact_level IN ('high', 'medium', 'low')),
    priority TEXT NOT NULL DEFAULT 'P4' CHECK (priority IN ('P1', 'P2', 'P3', 'P4', 'P5')),
    incident_type TEXT NOT NULL DEFAULT 'real' CHECK (incident_type IN ('real', 'alert', 'degradation')),
    commander_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    communicator_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    on_call_responder_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    commander_assignee_type TEXT NOT NULL DEFAULT 'user' CHECK (commander_assignee_type IN ('user', 'agent')),
    communicator_assignee_type TEXT NOT NULL DEFAULT 'user' CHECK (communicator_assignee_type IN ('user', 'agent')),
    service_id UUID NULL REFERENCES services(id) ON DELETE SET NULL,
    escalation_policy_id UUID NULL,
    conference_url TEXT NOT NULL DEFAULT '',
    slack_channel_id TEXT NULL,
    slack_channel_name TEXT NOT NULL DEFAULT '',
    slack_channel_archived BOOLEAN NOT NULL DEFAULT false,
    war_room_channel_id TEXT NULL,
    war_room_channel_provider TEXT NULL,
    google_meet_space_name TEXT NULL,
    status_page_incident_id TEXT NOT NULL DEFAULT '',
    sla_target_respond_at TIMESTAMPTZ NULL,
    sla_target_resolve_at TIMESTAMPTZ NULL,
    sla_acknowledged_at TIMESTAMPTZ NULL,
    sla_resolved_at TIMESTAMPTZ NULL,
    started_at TIMESTAMPTZ NULL,
    mitigated_at TIMESTAMPTZ NULL,
    resolved_at TIMESTAMPTZ NULL,
    closed_at TIMESTAMPTZ NULL,
    triaged_at TIMESTAMPTZ NULL,
    triage_report JSONB NULL,
    auto_confirmed BOOLEAN NOT NULL DEFAULT false,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    custom_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX incidents_status_created_at ON incidents (status, created_at) WHERE deleted_at IS NULL;
CREATE INDEX incidents_severity ON incidents (severity) WHERE deleted_at IS NULL;
CREATE INDEX incidents_priority ON incidents (priority) WHERE deleted_at IS NULL;
CREATE INDEX incidents_commander_id ON incidents (commander_id) WHERE deleted_at IS NULL;
CREATE INDEX incidents_communicator_id ON incidents (communicator_id) WHERE deleted_at IS NULL;
CREATE INDEX incidents_on_call_responder_id ON incidents (on_call_responder_id) WHERE deleted_at IS NULL;
CREATE INDEX incidents_service_id ON incidents (service_id) WHERE deleted_at IS NULL;
CREATE INDEX incidents_escalation_policy_id ON incidents (escalation_policy_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS incidents;
