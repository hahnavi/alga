-- Coordination Tasks are removed: per-incident agent work assignment flows
-- through the investigation pipeline and coordination-message activation.
-- The durable narrative survives in incident_coordination_messages (dispatch
-- and completion anchor messages keep task ids in their JSONB metadata) and
-- in incident timeline entries; the task rows themselves are dropped.

-- +goose Up
DROP INDEX IF EXISTS incident_coordination_messages_linked_coordination_task_id;
ALTER TABLE incident_coordination_messages DROP COLUMN IF EXISTS linked_coordination_task_id;
DROP TABLE IF EXISTS coordination_tasks;

-- +goose Down
CREATE TABLE coordination_tasks (
    id UUID PRIMARY KEY,
    incident_id UUID NULL REFERENCES incidents (id) ON DELETE SET NULL,
    parent_task_id UUID NULL,
    kind TEXT NOT NULL DEFAULT 'investigate' CHECK (kind IN ('investigate', 'communicate', 'verify', 'mitigate', 'synthesize')),
    assignee_role TEXT NOT NULL DEFAULT 'responder' CHECK (assignee_role IN ('commander', 'communicator', 'responder')),
    assignee_agent_id TEXT NULL DEFAULT '',
    assignee_agent_name TEXT NULL DEFAULT '',
    goal TEXT NOT NULL CHECK (goal <> ''),
    input_context JSONB NOT NULL DEFAULT '{}',
    result JSONB NULL,
    result_schema JSONB NULL,
    linked_investigation_id UUID NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'assigned', 'in_progress', 'complete', 'failed', 'cancelled')),
    priority INT NOT NULL DEFAULT 0 CHECK (priority >= 0),
    due_at TIMESTAMPTZ NULL,
    claimed_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    created_by_agent_id TEXT NULL DEFAULT '',
    created_by_name TEXT NULL DEFAULT '',
    failure_reason TEXT NULL DEFAULT '',
    dispatch_attempts INT NOT NULL DEFAULT 0 CHECK (dispatch_attempts >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT coordination_tasks_parent_task_id_fk FOREIGN KEY (parent_task_id) REFERENCES coordination_tasks (id) ON DELETE SET NULL
);
CREATE INDEX coordination_tasks_incident_id ON coordination_tasks (incident_id);
CREATE INDEX coordination_tasks_status_priority_created_at ON coordination_tasks (status, priority, created_at);
CREATE INDEX coordination_tasks_assignee_agent_id_status ON coordination_tasks (assignee_agent_id, status);
CREATE INDEX coordination_tasks_parent_task_id ON coordination_tasks (parent_task_id);
CREATE INDEX coordination_tasks_incident_id_status ON coordination_tasks (incident_id, status);
CREATE INDEX coordination_tasks_assignee_role_status ON coordination_tasks (assignee_role, status);
CREATE INDEX coordination_tasks_due_at ON coordination_tasks (due_at);
CREATE INDEX coordination_tasks_linked_investigation_id ON coordination_tasks (linked_investigation_id);
CREATE TRIGGER trg_coordination_tasks_set_updated_at BEFORE UPDATE ON coordination_tasks FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE incident_coordination_messages ADD COLUMN IF NOT EXISTS linked_coordination_task_id UUID NULL;
ALTER TABLE incident_coordination_messages ADD CONSTRAINT incident_coordination_messages_linked_task_id_fk FOREIGN KEY (linked_coordination_task_id) REFERENCES coordination_tasks (id) ON DELETE SET NULL;
CREATE INDEX incident_coordination_messages_linked_coordination_task_id ON incident_coordination_messages (linked_coordination_task_id);
