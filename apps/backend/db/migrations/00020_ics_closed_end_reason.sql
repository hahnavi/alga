-- +goose Up
-- WP-DT-E8: closing an incident tears down its live ICS role assignments with
-- the dedicated 'incident_closed' end reason (invoked handler-explicitly from
-- handleCloseIncident), so terminal incidents stop surfacing active
-- commanders/responders in ICS queries. Widening the CHECK keeps the manual
-- DELETE edge and the close-time teardown on the same enum.

ALTER TABLE ics_role_assignments DROP CONSTRAINT IF EXISTS ics_role_assignments_ended_reason_check;

ALTER TABLE ics_role_assignments ADD CONSTRAINT ics_role_assignments_ended_reason_check
    CHECK (ended_reason IN ('replaced', 'incident_resolved', 'assigned', 'agent_offline', 'incident_closed'));

-- +goose Down
-- Fold the close-time teardown rows into the closest surviving reason so the
-- 00006 constraint can be restored without failing on live data.
UPDATE ics_role_assignments SET ended_reason = 'incident_resolved' WHERE ended_reason = 'incident_closed';

ALTER TABLE ics_role_assignments DROP CONSTRAINT IF EXISTS ics_role_assignments_ended_reason_check;

ALTER TABLE ics_role_assignments ADD CONSTRAINT ics_role_assignments_ended_reason_check
    CHECK (ended_reason IN ('replaced', 'incident_resolved', 'assigned', 'agent_offline'));
