-- +goose Up
CREATE TABLE incident_investigation_updates (
    id UUID PRIMARY KEY,
    incident_investigation_id UUID NOT NULL,
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
    CONSTRAINT fk_incident_investigation_updates_investigation FOREIGN KEY (incident_investigation_id) REFERENCES incident_investigations(id) ON DELETE CASCADE
);

CREATE INDEX idx_incident_investigation_updates_created_at ON incident_investigation_updates (created_at);
CREATE INDEX idx_incident_investigation_updates_inv_id ON incident_investigation_updates (incident_investigation_id);

-- +goose Down
DROP TABLE IF EXISTS incident_investigation_updates;
