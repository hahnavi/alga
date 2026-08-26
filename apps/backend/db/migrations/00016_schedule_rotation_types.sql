-- +goose Up
-- Align schedule_layers.rotation_type with the resolver and the API/UI union:
-- hourly and monthly are implemented and offered, legacy `custom` always
-- behaved as weekly (resolver default branch), so it is rewritten to weekly.
ALTER TABLE schedule_layers DROP CONSTRAINT IF EXISTS schedule_layers_rotation_type_check;

UPDATE schedule_layers
SET rotation_type = 'weekly'
WHERE rotation_type NOT IN ('hourly', 'daily', 'weekly', 'monthly');

ALTER TABLE schedule_layers ADD CONSTRAINT schedule_layers_rotation_type_check
    CHECK (rotation_type IN ('hourly', 'daily', 'weekly', 'monthly'));

-- +goose Down
ALTER TABLE schedule_layers DROP CONSTRAINT IF EXISTS schedule_layers_rotation_type_check;

-- Fold values the old CHECK does not accept into its weekly equivalent, so
-- the original constraint can be restored without failing on existing rows.
UPDATE schedule_layers
SET rotation_type = 'weekly'
WHERE rotation_type NOT IN ('daily', 'weekly', 'custom');

ALTER TABLE schedule_layers ADD CONSTRAINT schedule_layers_rotation_type_check
    CHECK (rotation_type IN ('daily', 'weekly', 'custom'));
