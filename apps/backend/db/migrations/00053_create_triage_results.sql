-- +goose Up
CREATE TABLE triage_results (
    id UUID PRIMARY KEY,
    triage_number BIGINT NOT NULL UNIQUE CHECK (triage_number >= 0),
    correlation_key TEXT NOT NULL CHECK (correlation_key <> ''),
    alert_count INT NOT NULL DEFAULT 0,
    alert_fingerprints JSONB NULL,
    alert_labels JSONB NULL,
    severity_input TEXT NULL CHECK (severity_input IN ('critical', 'high', 'warning', 'info', 'low')),
    decision TEXT NOT NULL CHECK (decision IN ('investigate', 'auto_resolve', 'suppress', 'escalate', 'enrich_only')),
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    severity_classified TEXT NULL CHECK (severity_classified IN ('critical', 'high', 'warning', 'info', 'low')),
    category TEXT NULL CHECK (category IN ('infrastructure', 'application', 'network', 'security', 'other')),
    reasoning TEXT NULL DEFAULT '',
    suggested_actions JSONB NULL,
    enrichment JSONB NULL,
    context_used JSONB NULL,
    outcome TEXT NOT NULL DEFAULT 'pending' CHECK (outcome IN ('pending', 'confirmed', 'overridden')),
    overridden_to TEXT NULL CHECK (overridden_to IN ('investigate', 'auto_resolve', 'suppress', 'escalate', 'enrich_only')),
    overridden_by UUID NULL,
    overridden_at TIMESTAMPTZ NULL,
    model_used TEXT NULL DEFAULT '',
    triage_duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (triage_duration_ms >= 0),
    trace_id TEXT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT triage_results_overridden_by_fk FOREIGN KEY (overridden_by) REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX triage_results_correlation_key ON triage_results (correlation_key);
CREATE INDEX triage_results_decision ON triage_results (decision);
CREATE INDEX triage_results_outcome ON triage_results (outcome);
CREATE INDEX triage_results_created_at ON triage_results (created_at);
CREATE INDEX triage_results_overridden_by ON triage_results (overridden_by);

ALTER TABLE alert_investigations ADD CONSTRAINT fk_alert_investigations_triage_result FOREIGN KEY (triage_result_id) REFERENCES triage_results(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE IF EXISTS alert_investigations DROP CONSTRAINT IF EXISTS fk_alert_investigations_triage_result;
DROP TABLE IF EXISTS triage_results;
