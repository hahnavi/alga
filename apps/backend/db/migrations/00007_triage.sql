-- +goose Up

-- triage_results ------------------------------------------------------------
CREATE TABLE triage_results (
    id UUID PRIMARY KEY,
    triage_number BIGINT NOT NULL DEFAULT nextval('triage_number_seq') UNIQUE CHECK (triage_number >= 0),
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
    overridden_by UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    overridden_at TIMESTAMPTZ NULL,
    model_used TEXT NULL DEFAULT '',
    triage_duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (triage_duration_ms >= 0),
    trace_id TEXT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER SEQUENCE triage_number_seq OWNED BY triage_results.triage_number;

CREATE INDEX triage_results_correlation_key ON triage_results (correlation_key);
CREATE INDEX triage_results_decision ON triage_results (decision);
CREATE INDEX triage_results_outcome ON triage_results (outcome);
CREATE INDEX triage_results_created_at ON triage_results (created_at);
CREATE INDEX triage_results_overridden_by ON triage_results (overridden_by);
CREATE TRIGGER trg_triage_results_set_updated_at BEFORE UPDATE ON triage_results FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Deferred FK: alerts.triage_result_id -> triage_results (created above).
ALTER TABLE alerts
    ADD CONSTRAINT alerts_triage_result_id_fk
    FOREIGN KEY (triage_result_id) REFERENCES triage_results (id) ON DELETE SET NULL;

-- triage_rules --------------------------------------------------------------
CREATE TABLE triage_rules (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    description TEXT NULL DEFAULT '',
    conditions JSONB NULL,
    match_mode TEXT NOT NULL DEFAULT 'all' CHECK (match_mode IN ('all', 'any')),
    decision TEXT NOT NULL CHECK (decision IN ('investigate', 'auto_resolve', 'suppress', 'escalate', 'enrich_only')),
    severity TEXT NULL CHECK (severity IN ('critical', 'high', 'warning', 'info', 'low')),
    category TEXT NULL CHECK (category IN ('infrastructure', 'application', 'network', 'security', 'other')),
    enrichment JSONB NULL,
    priority INT NOT NULL DEFAULT 0 CHECK (priority >= 0),
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_by UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX triage_rules_enabled_priority ON triage_rules (enabled, priority);
CREATE INDEX triage_rules_created_by ON triage_rules (created_by);
CREATE TRIGGER trg_triage_rules_set_updated_at BEFORE UPDATE ON triage_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS triage_rules CASCADE;
ALTER TABLE alerts DROP CONSTRAINT IF EXISTS alerts_triage_result_id_fk;
DROP TABLE IF EXISTS triage_results CASCADE;
