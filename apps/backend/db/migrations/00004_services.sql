-- +goose Up

-- services ------------------------------------------------------------------
CREATE TABLE services (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    owner_team_id UUID NULL REFERENCES teams (id) ON DELETE SET NULL,
    escalation_policy_id UUID NULL REFERENCES escalation_policies (id) ON DELETE SET NULL,
    label_matchers JSONB NOT NULL DEFAULT '[]'::jsonb,
    sla_response_minutes INT NOT NULL DEFAULT 0 CHECK (sla_response_minutes >= 0),
    sla_resolve_minutes INT NOT NULL DEFAULT 0 CHECK (sla_resolve_minutes >= 0),
    status TEXT NOT NULL DEFAULT 'operational' CHECK (status IN ('operational', 'degraded', 'partial_outage', 'major_outage', 'maintenance')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX services_status ON services (status);
CREATE INDEX services_owner_team_id ON services (owner_team_id);
CREATE INDEX services_escalation_policy_id ON services (escalation_policy_id);
CREATE TRIGGER trg_services_set_updated_at BEFORE UPDATE ON services FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- service_dependencies ------------------------------------------------------
CREATE TABLE service_dependencies (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    dependent_on_service_id UUID NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    dependency_type TEXT NOT NULL DEFAULT 'depends_on' CHECK (dependency_type IN ('depends_on', 'hard', 'soft')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT service_dependencies_no_self CHECK (service_id <> dependent_on_service_id)
);
CREATE UNIQUE INDEX service_dependencies_service_id_dependent_on_service_id ON service_dependencies (service_id, dependent_on_service_id);
CREATE INDEX service_dependencies_dependent_on_service_id ON service_dependencies (dependent_on_service_id);

-- +goose Down
DROP TABLE IF EXISTS service_dependencies CASCADE;
DROP TABLE IF EXISTS services CASCADE;
