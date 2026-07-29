-- +goose Up
CREATE TABLE alert_investigation_updates (
    id UUID PRIMARY KEY,
    alert_investigation_id UUID NOT NULL,
    type TEXT NOT NULL,
    message TEXT NOT NULL,
    source TEXT NOT NULL,
    internal BOOLEAN NOT NULL DEFAULT false,
    edited BOOLEAN NOT NULL DEFAULT false,
    user_id TEXT,
    username TEXT,
    mm_post_id TEXT DEFAULT '',
    slack_message_ts TEXT DEFAULT '',
    quoted_update_id TEXT,
    mentions JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_alert_investigation_updates_investigation FOREIGN KEY (alert_investigation_id) REFERENCES alert_investigations(id) ON DELETE CASCADE
);

CREATE INDEX idx_alert_investigation_updates_created_at ON alert_investigation_updates (created_at);
CREATE INDEX idx_alert_investigation_updates_investigation_id ON alert_investigation_updates (alert_investigation_id);

-- +goose Down
DROP TABLE IF EXISTS alert_investigation_updates;
