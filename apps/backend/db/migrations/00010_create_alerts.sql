-- +goose Up
CREATE TABLE alerts (
    id UUID PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'firing' CHECK (status IN ('firing', 'resolved')),
    acknowledged BOOLEAN NOT NULL DEFAULT false,
    silenced BOOLEAN NOT NULL DEFAULT false,
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    annotations JSONB NOT NULL DEFAULT '{}'::jsonb,
    "values" JSONB NULL,
    starts_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ends_at TIMESTAMPTZ NULL,
    generator_url TEXT NOT NULL DEFAULT '',
    alert_number BIGINT NULL UNIQUE CHECK (alert_number >= 0),
    triage_result_id UUID NULL,
    enrichment JSONB NULL,
    triage_category TEXT NOT NULL DEFAULT '',
    severity_classified TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX alerts_fingerprint_updated_at ON alerts (fingerprint, updated_at);
CREATE UNIQUE INDEX alerts_fingerprint ON alerts (fingerprint) WHERE status != 'resolved' AND deleted_at IS NULL;
CREATE INDEX alerts_updated_at ON alerts (updated_at) WHERE deleted_at IS NULL;
CREATE INDEX alerts_status_created_at ON alerts (status, created_at) WHERE deleted_at IS NULL;
CREATE INDEX alerts_triage_result_id ON alerts (triage_result_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS alerts;
