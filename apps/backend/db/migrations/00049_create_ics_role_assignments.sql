-- +goose Up
CREATE TABLE ics_role_assignments (
    id UUID PRIMARY KEY,
    parent_id UUID NULL,
    incident_id UUID NOT NULL,
    user_id UUID NULL,
    agent_token_id UUID NULL,
    role_type TEXT NOT NULL DEFAULT 'responder' CHECK (role_type IN ('incident_commander', 'communications_lead', 'responder')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'ended')),
    assignee_type TEXT NOT NULL DEFAULT 'user' CHECK (assignee_type IN ('user', 'agent')),
    scope_description TEXT NULL,
    ended_reason TEXT NULL CHECK (ended_reason IN ('replaced', 'incident_resolved', 'assigned', 'agent_offline')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ NULL,
    CONSTRAINT ics_role_assignments_incident_id_fk FOREIGN KEY (incident_id) REFERENCES incidents (id) ON DELETE CASCADE,
    CONSTRAINT ics_role_assignments_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT ics_role_assignments_agent_token_id_fk FOREIGN KEY (agent_token_id) REFERENCES agent_tokens (id) ON DELETE SET NULL,
    CONSTRAINT ics_role_assignments_parent_id_fk FOREIGN KEY (parent_id) REFERENCES ics_role_assignments (id) ON DELETE SET NULL
);

CREATE INDEX ics_role_assignments_role_type ON ics_role_assignments (role_type);
CREATE INDEX ics_role_assignments_status ON ics_role_assignments (status);
CREATE INDEX ics_role_assignments_parent_id ON ics_role_assignments (parent_id);
CREATE INDEX ics_role_assignments_incident_id ON ics_role_assignments (incident_id);
CREATE INDEX ics_role_assignments_user_id ON ics_role_assignments (user_id);
CREATE INDEX ics_role_assignments_agent_token_id ON ics_role_assignments (agent_token_id);
CREATE UNIQUE INDEX ics_role_assignments_incident_id_role_type_active ON ics_role_assignments (incident_id, role_type) WHERE status = 'active';

-- +goose Down
DROP TABLE IF EXISTS ics_role_assignments;
