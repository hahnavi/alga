-- +goose Up
CREATE TABLE alert_investigation_events (
    id UUID PRIMARY KEY,
    alert_investigation_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('assigned', 'started', 'requeued', 'agent_offline_before_start', 'agent_offline_during_work', 'dispatch_failed', 'auto_completed', 'completed')),
    reason TEXT DEFAULT '',
    actor_type TEXT DEFAULT 'system',
    actor_id TEXT DEFAULT '',
    actor_name TEXT DEFAULT '',
    agent_id TEXT DEFAULT '',
    agent_name TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_alert_investigation_events_investigation FOREIGN KEY (alert_investigation_id) REFERENCES alert_investigations(id) ON DELETE CASCADE
);

CREATE INDEX idx_alert_investigation_events_inv_created ON alert_investigation_events (alert_investigation_id, created_at);
CREATE INDEX idx_alert_investigation_events_event_type ON alert_investigation_events (event_type);

-- +goose Down
DROP TABLE IF EXISTS alert_investigation_events;
