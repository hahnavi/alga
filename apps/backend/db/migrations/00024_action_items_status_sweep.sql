-- +goose Up
-- Action items: retire the legacy 'detected' status (rows migrate to 'open';
-- the workflow is open → in_progress → completed/cancelled) and index the
-- overdue sweep's predicate.

UPDATE action_items SET status = 'open' WHERE status = 'detected';

ALTER TABLE action_items DROP CONSTRAINT action_items_status_check;
ALTER TABLE action_items ADD CONSTRAINT action_items_status_check
    CHECK (status IN ('open', 'in_progress', 'completed', 'cancelled'));

-- Partial index matching ListOverdue: open items with a due date in the past.
CREATE INDEX action_items_overdue_sweep ON action_items (due_date)
    WHERE status NOT IN ('completed', 'cancelled') AND due_date IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS action_items_overdue_sweep;
ALTER TABLE action_items DROP CONSTRAINT action_items_status_check;
ALTER TABLE action_items ADD CONSTRAINT action_items_status_check
    CHECK (status IN ('open', 'detected', 'in_progress', 'completed', 'cancelled'));
