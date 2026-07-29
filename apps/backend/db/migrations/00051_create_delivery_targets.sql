-- +goose Up
CREATE TABLE delivery_targets (
    id UUID PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('slack', 'mattermost', 'pagerduty')),
    channel TEXT NOT NULL CHECK (channel <> ''),
    channel_name TEXT NULL DEFAULT '',
    post_id TEXT NULL DEFAULT '',
    alert_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT delivery_targets_alert_id_fk FOREIGN KEY (alert_id) REFERENCES alerts (id) ON DELETE CASCADE
);

CREATE INDEX delivery_targets_alert_id ON delivery_targets (alert_id);

-- +goose Down
DROP TABLE IF EXISTS delivery_targets;
