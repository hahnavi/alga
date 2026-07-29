-- +goose Up
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
    created_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT triage_rules_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX triage_rules_enabled_priority ON triage_rules (enabled, priority);
CREATE INDEX triage_rules_created_by ON triage_rules (created_by);

-- +goose Down
DROP TABLE IF EXISTS triage_rules;
