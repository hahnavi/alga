-- +goose Up

-- alerts --------------------------------------------------------------------
-- alert_number is the true unique alert identifier (numeric, load-bearing in
-- API + frontend). It is now assigned by a Postgres sequence via DEFAULT.
-- fingerprint is a dedup key (NOT unique); the partial unique index enforces
-- "one open alert per fingerprint" and must remain byte-identical.
CREATE TABLE alerts (
    id UUID PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'firing' CHECK (status IN ('firing', 'resolved')),
    acknowledged BOOLEAN NOT NULL DEFAULT false,
    silenced BOOLEAN NOT NULL DEFAULT false,
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    annotations JSONB NOT NULL DEFAULT '{}'::jsonb,
    "values" JSONB NULL,
    starts_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ends_at TIMESTAMPTZ NULL,
    generator_url TEXT NOT NULL DEFAULT '',
    alert_number BIGINT NOT NULL DEFAULT nextval('alert_number_seq') UNIQUE CHECK (alert_number >= 0),
    triage_result_id UUID NULL,
    enrichment JSONB NULL,
    triage_category TEXT NOT NULL DEFAULT '',
    severity_classified TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
ALTER SEQUENCE alert_number_seq OWNED BY alerts.alert_number;

CREATE INDEX alerts_fingerprint_updated_at ON alerts (fingerprint, updated_at);
CREATE UNIQUE INDEX alerts_fingerprint ON alerts (fingerprint) WHERE status != 'resolved' AND deleted_at IS NULL;
CREATE INDEX alerts_updated_at ON alerts (updated_at) WHERE deleted_at IS NULL;
CREATE INDEX alerts_status_created_at ON alerts (status, created_at) WHERE deleted_at IS NULL;
CREATE INDEX alerts_triage_result_id ON alerts (triage_result_id) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_alerts_set_updated_at BEFORE UPDATE ON alerts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- alert_events --------------------------------------------------------------
-- Append-only. `timestamp` is the authoritative event time (read widely);
-- the previously redundant created_at/updated_at columns are dropped.
-- type/source are open-ended free-text and intentionally left unconstrained.
CREATE TABLE alert_events (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_username TEXT NOT NULL DEFAULT '',
    actor_display_name TEXT NOT NULL DEFAULT '',
    actor_user_id TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    alert_id UUID NOT NULL REFERENCES alerts (id) ON DELETE CASCADE
);
CREATE INDEX alert_events_alert_id_timestamp ON alert_events (alert_id, timestamp);

-- delivery_targets ----------------------------------------------------------
CREATE TABLE delivery_targets (
    id UUID PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('slack', 'mattermost', 'pagerduty')),
    channel TEXT NOT NULL CHECK (channel <> ''),
    channel_name TEXT NULL DEFAULT '',
    post_id TEXT NULL DEFAULT '',
    alert_id UUID NOT NULL REFERENCES alerts (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX delivery_targets_alert_id ON delivery_targets (alert_id);
CREATE TRIGGER trg_delivery_targets_set_updated_at BEFORE UPDATE ON delivery_targets FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS delivery_targets CASCADE;
DROP TABLE IF EXISTS alert_events CASCADE;
DROP TABLE IF EXISTS alerts CASCADE;
