-- +goose Up
-- WP-A2: the test-notification endpoint (07_notifications/03 R16) inserts
-- type='test' / resource_type='system', which every prior CHECK generation
-- rejects, so POST /users/me/notification-preferences/test always failed with
-- a 500 on conforming schemas. Widen the enums to the shipped endpoint
-- contract instead of remapping the endpoint's type. Delivery-log parity
-- follows the 00017 lockstep pattern.

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_type_check;

ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
    CHECK (type IN (
        'escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info',
        'incident_acknowledged', 'incident_mitigated', 'incident_resolved', 'incident_reopened',
        'oncall_reminder', 'test'
    ));

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_resource_type_check;

ALTER TABLE notifications ADD CONSTRAINT notifications_resource_type_check
    CHECK (resource_type IN ('incident', 'investigation', 'post_mortem', 'action_item', 'alert', 'handoff', 'schedule', 'system'));

ALTER TABLE notification_delivery_logs DROP CONSTRAINT IF EXISTS notification_delivery_logs_notification_type_check;

ALTER TABLE notification_delivery_logs ADD CONSTRAINT notification_delivery_logs_notification_type_check
    CHECK (notification_type IN (
        'escalation', 'oncall_handoff', 'post_mortem_review_requested', 'action_item_assigned', 'mention', 'info',
        'incident_acknowledged', 'incident_mitigated', 'incident_resolved', 'incident_reopened',
        'oncall_reminder', 'test'
    ));

-- +goose Down
-- Fold the widened values back into their closest surviving equivalents so the
-- 00017 constraints can be restored without failing on live data.
UPDATE notifications SET type = 'info' WHERE type = 'test';

UPDATE notification_delivery_logs SET notification_type = 'info' WHERE notification_type = 'test';

UPDATE notifications SET resource_type = NULL WHERE resource_type = 'system';

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
