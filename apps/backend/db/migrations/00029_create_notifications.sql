-- +goose Up
CREATE TABLE notifications (
    id UUID PRIMARY KEY,
    user_id TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info')),
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    read BOOLEAN NOT NULL DEFAULT false,
    resource_type TEXT CHECK (resource_type IN ('incident', 'investigation', 'post_mortem', 'action_item')),
    resource_id TEXT DEFAULT '',
    triggered_by_user_id TEXT DEFAULT '',
    triggered_by_display_name TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_user_id_created_at ON notifications (user_id, created_at);
CREATE INDEX idx_notifications_user_id_read ON notifications (user_id, read);

-- +goose Down
DROP TABLE IF EXISTS notifications;
