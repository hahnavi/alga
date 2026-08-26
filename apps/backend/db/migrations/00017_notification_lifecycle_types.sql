-- +goose Up
-- WP-C4: phase-1 notification producers ship with real, distinct types instead
-- of collapsing into `info`. Four incident lifecycle transitions aimed at the
-- commander/responders plus an ahead-of-handoff shift reminder join the enum;
-- resource_type gains the values shipped producers actually set (`alert` for
-- thread mentions, `handoff` for handoff records, `schedule` for reminders).

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_type_check;

ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
    CHECK (type IN (
        'escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info',
        'incident_acknowledged', 'incident_mitigated', 'incident_resolved', 'incident_reopened',
        'oncall_reminder'
    ));

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_resource_type_check;

ALTER TABLE notifications ADD CONSTRAINT notifications_resource_type_check
    CHECK (resource_type IN ('incident', 'investigation', 'post_mortem', 'action_item', 'alert', 'handoff', 'schedule'));

ALTER TABLE notification_delivery_logs DROP CONSTRAINT IF EXISTS notification_delivery_logs_notification_type_check;

ALTER TABLE notification_delivery_logs ADD CONSTRAINT notification_delivery_logs_notification_type_check
    CHECK (notification_type IN (
        'escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info',
        'incident_acknowledged', 'incident_mitigated', 'incident_resolved', 'incident_reopened',
        'oncall_reminder'
    ));

-- +goose Down
-- Fold widened values back into their closest surviving equivalents so the
-- original 00010 constraints can be restored without failing on live data.
UPDATE notifications SET type = 'info'
WHERE type IN ('oncall_reminder', 'incident_acknowledged', 'incident_mitigated', 'incident_resolved', 'incident_reopened');

UPDATE notification_delivery_logs SET notification_type = 'info'
WHERE notification_type IN ('oncall_reminder', 'incident_acknowledged', 'incident_mitigated', 'incident_resolved', 'incident_reopened');

UPDATE notifications SET resource_type = NULL
WHERE resource_type IN ('alert', 'handoff', 'schedule');

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_type_check;

ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
    CHECK (type IN ('escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info'));

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_resource_type_check;

ALTER TABLE notifications ADD CONSTRAINT notifications_resource_type_check
    CHECK (resource_type IN ('incident', 'investigation', 'post_mortem', 'action_item'));

ALTER TABLE notification_delivery_logs DROP CONSTRAINT IF EXISTS notification_delivery_logs_notification_type_check;

ALTER TABLE notification_delivery_logs ADD CONSTRAINT notification_delivery_logs_notification_type_check
    CHECK (notification_type IN ('escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info'));
