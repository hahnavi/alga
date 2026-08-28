-- +goose Up
-- remove vestigial 'pagerduty' enum value from delivery_targets.provider.
-- No producer or consumer exists anywhere in the codebase; zero rows can carry
-- the value (defensive DELETE guards against hand-inserted data).
DELETE FROM delivery_targets WHERE provider = 'pagerduty';

ALTER TABLE delivery_targets
    DROP CONSTRAINT IF EXISTS delivery_targets_provider_check;
ALTER TABLE delivery_targets
    ADD CONSTRAINT delivery_targets_provider_check
    CHECK (provider IN ('slack', 'mattermost'));

-- +goose Down
-- Authoring documentation / manual recovery script only: the binary executes
-- up-blocks exclusively (no down CLI path exists).
ALTER TABLE delivery_targets
    DROP CONSTRAINT IF EXISTS delivery_targets_provider_check;
ALTER TABLE delivery_targets
    ADD CONSTRAINT delivery_targets_provider_check
    CHECK (provider IN ('slack', 'mattermost', 'pagerduty'));
