-- +goose Up
-- Investigation dispatch leases: an assignment is valid until lease_until; the
-- scheduler requeues rows whose lease expired. Backfill gives in-flight rows one
-- default timeout window (10m) from their last activity so a deploy does not
-- instantly requeue agents that are actively working.
ALTER TABLE alert_investigations ADD COLUMN lease_until TIMESTAMPTZ;
ALTER TABLE incident_investigations ADD COLUMN lease_until TIMESTAMPTZ;

UPDATE alert_investigations
SET lease_until = updated_at + interval '10 minutes'
WHERE status IN ('assigned', 'investigating');

UPDATE incident_investigations
SET lease_until = updated_at + interval '10 minutes'
WHERE status IN ('assigned', 'investigating');

CREATE INDEX idx_alert_investigations_lease_expire
    ON alert_investigations (lease_until)
    WHERE status IN ('assigned', 'investigating');

CREATE INDEX idx_incident_investigations_lease_expire
    ON incident_investigations (lease_until)
    WHERE status IN ('assigned', 'investigating');

-- +goose Down
DROP INDEX IF EXISTS idx_incident_investigations_lease_expire;
DROP INDEX IF EXISTS idx_alert_investigations_lease_expire;
ALTER TABLE incident_investigations DROP COLUMN IF EXISTS lease_until;
ALTER TABLE alert_investigations DROP COLUMN IF EXISTS lease_until;
