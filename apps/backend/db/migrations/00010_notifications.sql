-- +goose Up

-- notifications -------------------------------------------------------------
-- user_id and triggered_by_user_id are now real UUID foreign keys to users
-- (previously loose TEXT). Append-oriented: only created_at is tracked.
CREATE TABLE notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info')),
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    read BOOLEAN NOT NULL DEFAULT false,
    resource_type TEXT CHECK (resource_type IN ('incident', 'investigation', 'post_mortem', 'action_item')),
    resource_id TEXT NOT NULL DEFAULT '',
    triggered_by_user_id UUID,
    triggered_by_display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT notifications_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT notifications_triggered_by_user_id_fk FOREIGN KEY (triggered_by_user_id) REFERENCES users (id) ON DELETE SET NULL
);
CREATE INDEX notifications_user_id_created_at ON notifications (user_id, created_at);
CREATE INDEX notifications_user_id_read ON notifications (user_id, read);

-- notification_delivery_logs ------------------------------------------------
-- Append-only delivery audit trail. user_id / incident_id are now real FKs.
CREATE TABLE notification_delivery_logs (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    incident_id UUID,
    notification_type TEXT NOT NULL CHECK (notification_type IN ('escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info')),
    channel TEXT NOT NULL CHECK (channel IN ('email', 'mattermost', 'slack', 'voice')),
    status TEXT NOT NULL DEFAULT 'sent' CHECK (status IN ('sent', 'delivered', 'failed', 'queued', 'skipped', 'skipped_no_slack_id', 'skipped_no_phone', 'skipped_opt_out', 'skipped_dedup')),
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT notification_delivery_logs_user_id_fk FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT notification_delivery_logs_incident_id_fk FOREIGN KEY (incident_id) REFERENCES incidents (id) ON DELETE SET NULL
);
CREATE INDEX notification_delivery_logs_user_id_created_at ON notification_delivery_logs (user_id, created_at);
CREATE INDEX notification_delivery_logs_incident_id ON notification_delivery_logs (incident_id);
CREATE INDEX notification_delivery_logs_status ON notification_delivery_logs (status);

-- +goose Down
DROP TABLE IF EXISTS notification_delivery_logs CASCADE;
DROP TABLE IF EXISTS notifications CASCADE;
