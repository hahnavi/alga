-- +goose Up
CREATE TABLE alert_investigation_alerts (
    id UUID PRIMARY KEY,
    alert_investigation_id UUID NOT NULL,
    alert_id UUID,
    fingerprint TEXT NOT NULL,
    alert_number BIGINT CHECK (alert_number >= 0),
    status TEXT DEFAULT '',
    alertname TEXT DEFAULT '',
    namespace TEXT DEFAULT '',
    labels JSONB,
    annotations JSONB,
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    generator_url TEXT DEFAULT '',
    summary TEXT DEFAULT '',
    current BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_alert_investigation_alerts_investigation FOREIGN KEY (alert_investigation_id) REFERENCES alert_investigations(id) ON DELETE CASCADE,
    CONSTRAINT fk_alert_investigation_alerts_alert FOREIGN KEY (alert_id) REFERENCES alerts(id) ON DELETE SET NULL
);

CREATE INDEX idx_alert_investigation_alerts_fingerprint ON alert_investigation_alerts (fingerprint);
CREATE INDEX idx_alert_investigation_alerts_investigation_id ON alert_investigation_alerts (alert_investigation_id);
CREATE UNIQUE INDEX idx_alert_investigation_alerts_alert_number_current ON alert_investigation_alerts (alert_number, current) WHERE current = true AND alert_number > 0;
CREATE UNIQUE INDEX idx_alert_investigation_alerts_alert_id_current ON alert_investigation_alerts (alert_id, current) WHERE current = true AND alert_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS alert_investigation_alerts;
