-- +goose Up
CREATE TABLE incident_investigations (
    id UUID PRIMARY KEY,
    incident_investigation_id TEXT NOT NULL UNIQUE,
    incident_id UUID,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'assigned', 'investigating', 'paused', 'complete', 'cancelled', 'coordinating')),
    agent_id TEXT DEFAULT '',
    agent_name TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    primary_thread_id TEXT DEFAULT '',
    slack_channel_id TEXT DEFAULT '',
    slack_thread_ts TEXT DEFAULT '',
    mm_post_id TEXT DEFAULT '',
    mm_thread_id TEXT DEFAULT '',
    source_alert_investigation_id UUID,
    summary JSONB,
    findings JSONB,
    evidence JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    investigating_duration_ms BIGINT DEFAULT 0,
    parent_investigation_id UUID,
    assignee_type TEXT NOT NULL DEFAULT 'agent' CHECK (assignee_type IN ('agent', 'user', 'system', 'grafana')),
    assignee_id UUID,
    CONSTRAINT fk_incident_investigations_incident FOREIGN KEY (incident_id) REFERENCES incidents(id) ON DELETE SET NULL,
    CONSTRAINT fk_incident_investigations_source_alert_inv FOREIGN KEY (source_alert_investigation_id) REFERENCES alert_investigations(id) ON DELETE SET NULL,
    CONSTRAINT fk_incident_investigations_parent FOREIGN KEY (parent_investigation_id) REFERENCES incident_investigations(id) ON DELETE SET NULL
);

CREATE INDEX idx_incident_investigations_status_created_at ON incident_investigations (status, created_at);
CREATE INDEX idx_incident_investigations_source_alert_inv_id ON incident_investigations (source_alert_investigation_id);
CREATE INDEX idx_incident_investigations_incident_id_status ON incident_investigations (incident_id, status);
CREATE INDEX idx_incident_investigations_parent_id ON incident_investigations (parent_investigation_id);
CREATE INDEX idx_incident_investigations_assignee ON incident_investigations (assignee_type, assignee_id, status);

-- Add deferred FK from alert_investigations now that incident_investigations exists
ALTER TABLE alert_investigations
    ADD CONSTRAINT fk_alert_investigations_promoted_incident_inv
    FOREIGN KEY (promoted_incident_investigation_id) REFERENCES incident_investigations(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE alert_investigations DROP CONSTRAINT IF EXISTS fk_alert_investigations_promoted_incident_inv;
DROP TABLE IF EXISTS incident_investigations;
