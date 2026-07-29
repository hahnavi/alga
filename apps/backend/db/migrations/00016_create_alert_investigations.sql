-- +goose Up
CREATE TABLE alert_investigations (
    id UUID PRIMARY KEY,
    alert_investigation_id TEXT NOT NULL UNIQUE,
    correlation_key TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'assigned', 'investigating', 'promoted', 'complete', 'failed', 'cancelled', 'timed_out', 'paused')),
    agent_id TEXT DEFAULT '',
    agent_name TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    primary_thread_id TEXT DEFAULT '',
    slack_channel_id TEXT DEFAULT '',
    slack_thread_ts TEXT DEFAULT '',
    mm_post_id TEXT DEFAULT '',
    mm_thread_id TEXT DEFAULT '',
    promoted_incident_id UUID,
    promoted_incident_investigation_id UUID,
    summary JSONB,
    findings JSONB,
    evidence JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    completed_reason TEXT DEFAULT '',
    completed_by_type TEXT DEFAULT '',
    completed_by_id TEXT DEFAULT '',
    completed_by_name TEXT DEFAULT '',
    started_at TIMESTAMPTZ,
    investigating_duration_ms BIGINT DEFAULT 0,
    primary_alert_fingerprint TEXT NOT NULL DEFAULT '',
    primary_alert_number BIGINT CHECK (primary_alert_number >= 0),
    escalation_level TEXT DEFAULT '',
    triage_result_id UUID,
    triage_decision TEXT DEFAULT '',
    triage_enrichment JSONB,
    assignee_type TEXT NOT NULL DEFAULT 'agent' CHECK (assignee_type IN ('agent', 'user', 'system', 'grafana')),
    assignee_id UUID,
    CONSTRAINT fk_alert_investigations_promoted_incident FOREIGN KEY (promoted_incident_id) REFERENCES incidents(id) ON DELETE SET NULL
);

CREATE INDEX idx_alert_investigations_status_created_at ON alert_investigations (status, created_at);
CREATE INDEX idx_alert_investigations_correlation_key_status ON alert_investigations (correlation_key, status);
CREATE INDEX idx_alert_investigations_promoted_incident_id ON alert_investigations (promoted_incident_id);
CREATE INDEX idx_alert_investigations_promoted_incident_inv_id ON alert_investigations (promoted_incident_investigation_id);
CREATE INDEX idx_alert_investigations_primary_alert_fingerprint ON alert_investigations (primary_alert_fingerprint);
CREATE INDEX idx_alert_investigations_primary_alert_number ON alert_investigations (primary_alert_number);
CREATE INDEX idx_alert_investigations_triage_result_id ON alert_investigations (triage_result_id);
CREATE INDEX idx_alert_investigations_assignee ON alert_investigations (assignee_type, assignee_id, status);

-- +goose Down
DROP TABLE IF EXISTS alert_investigations;
