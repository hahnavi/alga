-- +goose Up
CREATE TABLE notification_delivery_logs (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    incident_id UUID,
    notification_type TEXT NOT NULL CHECK (notification_type IN ('escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info')),
    channel TEXT NOT NULL CHECK (channel IN ('email', 'mattermost', 'slack', 'voice')),
    status TEXT NOT NULL DEFAULT 'sent' CHECK (status IN ('sent', 'delivered', 'failed', 'queued', 'skipped', 'skipped_no_slack_id', 'skipped_no_phone', 'skipped_opt_out', 'skipped_dedup')),
    error_message TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notification_delivery_logs_user_id_created_at ON notification_delivery_logs (user_id, created_at);
CREATE INDEX idx_notification_delivery_logs_incident_id ON notification_delivery_logs (incident_id);
CREATE INDEX idx_notification_delivery_logs_status ON notification_delivery_logs (status);

-- +goose Down
DROP TABLE IF EXISTS notification_delivery_logs;
