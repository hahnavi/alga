-- +goose Up
CREATE TABLE alert_events (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_username TEXT NOT NULL DEFAULT '',
    actor_display_name TEXT NOT NULL DEFAULT '',
    actor_user_id TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX alert_events_alert_id_timestamp ON alert_events (alert_id, timestamp);

-- +goose Down
DROP TABLE IF EXISTS alert_events;
