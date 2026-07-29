-- +goose Up
CREATE TABLE services (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    owner_team_id UUID NULL,
    escalation_policy_id UUID NULL,
    label_matchers JSONB NOT NULL DEFAULT '[]'::jsonb,
    sla_response_minutes INT NOT NULL DEFAULT 0 CHECK (sla_response_minutes >= 0),
    sla_resolve_minutes INT NOT NULL DEFAULT 0 CHECK (sla_resolve_minutes >= 0),
    status TEXT NOT NULL DEFAULT 'operational' CHECK (status IN ('operational', 'degraded', 'partial_outage', 'major_outage', 'maintenance')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX services_status ON services (status);
CREATE INDEX services_owner_team_id ON services (owner_team_id);
CREATE INDEX services_escalation_policy_id ON services (escalation_policy_id);

-- +goose Down
DROP TABLE IF EXISTS services;
