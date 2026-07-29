-- +goose Up

-- incidents -----------------------------------------------------------------
-- incident_number is numeric and load-bearing (API + frontend). Assigned via
-- sequence; ReserveIncidentNumber allocates via nextval() before row insert.
CREATE TABLE incidents (
    id UUID PRIMARY KEY,
    incident_number BIGINT NOT NULL DEFAULT nextval('incident_number_seq') UNIQUE CHECK (incident_number >= 0),
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'detected' CHECK (status IN ('detected', 'triaging', 'active', 'mitigated', 'resolved', 'closed', 'cancelled')),
    severity TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('critical', 'high', 'warning', 'info')),
    impact_level TEXT NOT NULL DEFAULT 'medium' CHECK (impact_level IN ('high', 'medium', 'low')),
    priority TEXT NOT NULL DEFAULT 'P4' CHECK (priority IN ('P1', 'P2', 'P3', 'P4', 'P5')),
    incident_type TEXT NOT NULL DEFAULT 'real' CHECK (incident_type IN ('real', 'alert', 'degradation')),
    commander_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    communicator_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    on_call_responder_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    commander_assignee_type TEXT NOT NULL DEFAULT 'user' CHECK (commander_assignee_type IN ('user', 'agent')),
    communicator_assignee_type TEXT NOT NULL DEFAULT 'user' CHECK (communicator_assignee_type IN ('user', 'agent')),
    service_id UUID NULL REFERENCES services (id) ON DELETE SET NULL,
    escalation_policy_id UUID NULL REFERENCES escalation_policies (id) ON DELETE SET NULL,
    conference_url TEXT NOT NULL DEFAULT '',
    slack_channel_id TEXT NULL,
    slack_channel_name TEXT NOT NULL DEFAULT '',
    slack_channel_archived BOOLEAN NOT NULL DEFAULT false,
    war_room_channel_id TEXT NULL,
    war_room_channel_provider TEXT NULL,
    google_meet_space_name TEXT NULL,
    status_page_incident_id TEXT NOT NULL DEFAULT '',
    sla_target_respond_at TIMESTAMPTZ NULL,
    sla_target_resolve_at TIMESTAMPTZ NULL,
    sla_acknowledged_at TIMESTAMPTZ NULL,
    sla_resolved_at TIMESTAMPTZ NULL,
    started_at TIMESTAMPTZ NULL,
    mitigated_at TIMESTAMPTZ NULL,
    resolved_at TIMESTAMPTZ NULL,
    closed_at TIMESTAMPTZ NULL,
    triaged_at TIMESTAMPTZ NULL,
    triage_report JSONB NULL,
    auto_confirmed BOOLEAN NOT NULL DEFAULT false,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    custom_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
ALTER SEQUENCE incident_number_seq OWNED BY incidents.incident_number;

CREATE INDEX incidents_status_created_at ON incidents (status, created_at) WHERE deleted_at IS NULL;
CREATE INDEX incidents_severity ON incidents (severity) WHERE deleted_at IS NULL;
CREATE INDEX incidents_priority ON incidents (priority) WHERE deleted_at IS NULL;
CREATE INDEX incidents_commander_id ON incidents (commander_id) WHERE deleted_at IS NULL;
CREATE INDEX incidents_communicator_id ON incidents (communicator_id) WHERE deleted_at IS NULL;
CREATE INDEX incidents_on_call_responder_id ON incidents (on_call_responder_id) WHERE deleted_at IS NULL;
CREATE INDEX incidents_service_id ON incidents (service_id) WHERE deleted_at IS NULL;
CREATE INDEX incidents_escalation_policy_id ON incidents (escalation_policy_id) WHERE deleted_at IS NULL;
CREATE TRIGGER trg_incidents_set_updated_at BEFORE UPDATE ON incidents FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- coordination_tasks --------------------------------------------------------
-- assignee_agent_id / created_by_agent_id are TEXT with an empty-string
-- sentinel (Set("agent_id = ''")); they intentionally stay TEXT.
-- The FK linked_investigation_id -> alert_investigations is added in 00008
-- because alert_investigations is created there.
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

-- incident_documents --------------------------------------------------------
CREATE TABLE incident_documents (
    id UUID PRIMARY KEY,
    section TEXT NOT NULL DEFAULT 'current_status',
    content TEXT NOT NULL DEFAULT '',
    version INT NOT NULL DEFAULT 1,
    incident_id UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    updated_by_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX incident_documents_incident_id_section ON incident_documents (incident_id, section);
CREATE INDEX incident_documents_updated_by_id ON incident_documents (updated_by_id);
CREATE TRIGGER trg_incident_documents_set_updated_at BEFORE UPDATE ON incident_documents FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- incident_alerts (join) ----------------------------------------------------
CREATE TABLE incident_alerts (
    incident_id UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    alert_id UUID NOT NULL REFERENCES alerts (id) ON DELETE CASCADE,
    PRIMARY KEY (incident_id, alert_id)
);
CREATE INDEX idx_incident_alerts_alert_id ON incident_alerts (alert_id);

-- incident_timeline_entries (append-only) -----------------------------------
CREATE TABLE incident_timeline_entries (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL DEFAULT 'note_added',
    actor_id UUID NULL,
    actor_type TEXT NOT NULL DEFAULT 'system',
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ics_event_type TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    incident_id UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE
);
CREATE INDEX incident_timeline_entries_incident_id_created_at ON incident_timeline_entries (incident_id, created_at);

-- incident_coordination_messages --------------------------------------------
CREATE TABLE incident_coordination_messages (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL DEFAULT 'chat',
    actor_type TEXT NOT NULL DEFAULT 'system',
    actor_id UUID NULL,
    actor_display_name TEXT NULL DEFAULT '',
    body TEXT NOT NULL CHECK (body <> ''),
    internal BOOLEAN NOT NULL DEFAULT false,
    source TEXT NOT NULL DEFAULT 'alga',
    slack_channel_id TEXT NULL DEFAULT '',
    slack_message_ts TEXT NULL DEFAULT '',
    slack_thread_ts TEXT NULL DEFAULT '',
    provider_message_id TEXT NULL DEFAULT '',
    linked_investigation_id TEXT NULL DEFAULT '',
    parent_message_id UUID NULL,
    linked_coordination_task_id UUID NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    incident_id UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    CONSTRAINT incident_coordination_messages_parent_message_id_fk FOREIGN KEY (parent_message_id) REFERENCES incident_coordination_messages (id) ON DELETE SET NULL,
    CONSTRAINT incident_coordination_messages_linked_task_id_fk FOREIGN KEY (linked_coordination_task_id) REFERENCES coordination_tasks (id) ON DELETE SET NULL
);
CREATE INDEX incident_coordination_messages_created_at ON incident_coordination_messages (created_at);
CREATE INDEX incident_coordination_messages_provider_message_id ON incident_coordination_messages (provider_message_id);
CREATE INDEX incident_coordination_messages_slack_channel_id_slack_message_ts ON incident_coordination_messages (slack_channel_id, slack_message_ts);
CREATE INDEX incident_coordination_messages_parent_message_id ON incident_coordination_messages (parent_message_id);
CREATE INDEX incident_coordination_messages_linked_coordination_task_id ON incident_coordination_messages (linked_coordination_task_id);
CREATE INDEX incident_coordination_messages_incident_id_created_at ON incident_coordination_messages (incident_id, created_at);
CREATE TRIGGER trg_incident_coordination_messages_set_updated_at BEFORE UPDATE ON incident_coordination_messages FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ics_role_assignments ------------------------------------------------------
CREATE TABLE ics_role_assignments (
    id UUID PRIMARY KEY,
    parent_id UUID NULL,
    incident_id UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    user_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    agent_token_id UUID NULL REFERENCES agent_tokens (id) ON DELETE SET NULL,
    role_type TEXT NOT NULL DEFAULT 'responder' CHECK (role_type IN ('incident_commander', 'communications_lead', 'responder')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'ended')),
    assignee_type TEXT NOT NULL DEFAULT 'user' CHECK (assignee_type IN ('user', 'agent')),
    scope_description TEXT NULL,
    ended_reason TEXT NULL CHECK (ended_reason IN ('replaced', 'incident_resolved', 'assigned', 'agent_offline')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ NULL,
    CONSTRAINT ics_role_assignments_parent_id_fk FOREIGN KEY (parent_id) REFERENCES ics_role_assignments (id) ON DELETE SET NULL
);
CREATE INDEX ics_role_assignments_role_type ON ics_role_assignments (role_type);
CREATE INDEX ics_role_assignments_status ON ics_role_assignments (status);
CREATE INDEX ics_role_assignments_parent_id ON ics_role_assignments (parent_id);
CREATE INDEX ics_role_assignments_incident_id ON ics_role_assignments (incident_id);
CREATE INDEX ics_role_assignments_user_id ON ics_role_assignments (user_id);
CREATE INDEX ics_role_assignments_agent_token_id ON ics_role_assignments (agent_token_id);
CREATE UNIQUE INDEX ics_role_assignments_incident_id_role_type_active ON ics_role_assignments (incident_id, role_type) WHERE status = 'active';

-- handoff_records -----------------------------------------------------------
CREATE TABLE handoff_records (
    id UUID PRIMARY KEY,
    schedule_id UUID NOT NULL REFERENCES on_call_schedules (id) ON DELETE CASCADE,
    outgoing_user_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    incoming_user_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    handoff_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'acknowledged')),
    outgoing_notes TEXT NULL,
    incoming_notes TEXT NULL,
    incoming_acknowledged_at TIMESTAMPTZ NULL,
    incident_summary TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX handoff_records_schedule_id_handoff_at ON handoff_records (schedule_id, handoff_at);
CREATE INDEX handoff_records_incoming_user_id_status ON handoff_records (incoming_user_id, status);
CREATE INDEX handoff_records_outgoing_user_id ON handoff_records (outgoing_user_id);
CREATE INDEX handoff_records_status_handoff_at ON handoff_records (status, handoff_at);
CREATE TRIGGER trg_handoff_records_set_updated_at BEFORE UPDATE ON handoff_records FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- post_mortems --------------------------------------------------------------
CREATE TABLE post_mortems (
    id UUID PRIMARY KEY,
    incident_id UUID NOT NULL UNIQUE REFERENCES incidents (id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'approved', 'published')),
    summary TEXT NOT NULL DEFAULT '',
    timeline JSONB NULL,
    root_cause TEXT NOT NULL DEFAULT '',
    contributing_factors JSONB NULL,
    impact TEXT NOT NULL DEFAULT '',
    lessons_learned TEXT NOT NULL DEFAULT '',
    what_went_well TEXT NOT NULL DEFAULT '',
    what_went_wrong TEXT NOT NULL DEFAULT '',
    blameless_confirmed BOOLEAN NOT NULL DEFAULT false,
    blameless_notes TEXT NOT NULL DEFAULT '',
    approved_by_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    published_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX post_mortems_incident_id ON post_mortems (incident_id);
CREATE INDEX post_mortems_status_created_at ON post_mortems (status, created_at);
CREATE INDEX post_mortems_approved_by_id ON post_mortems (approved_by_id);
CREATE TRIGGER trg_post_mortems_set_updated_at BEFORE UPDATE ON post_mortems FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- action_items --------------------------------------------------------------
CREATE TABLE action_items (
    id UUID PRIMARY KEY,
    post_mortem_id UUID NOT NULL REFERENCES post_mortems (id) ON DELETE CASCADE,
    description TEXT NOT NULL CHECK (description <> ''),
    type TEXT NOT NULL DEFAULT 'investigate' CHECK (type IN ('prevent', 'mitigate', 'detect', 'investigate')),
    assignee_name TEXT NULL DEFAULT '',
    assignee_id UUID NULL REFERENCES users (id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'detected', 'in_progress', 'completed', 'cancelled')),
    priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high')),
    due_date TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX action_items_post_mortem_id ON action_items (post_mortem_id);
CREATE INDEX action_items_assignee_id_status ON action_items (assignee_id, status);
CREATE INDEX action_items_status ON action_items (status);
CREATE INDEX action_items_type ON action_items (type);
CREATE TRIGGER trg_action_items_set_updated_at BEFORE UPDATE ON action_items FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS action_items CASCADE;
DROP TABLE IF EXISTS post_mortems CASCADE;
DROP TABLE IF EXISTS handoff_records CASCADE;
DROP TABLE IF EXISTS ics_role_assignments CASCADE;
DROP TABLE IF EXISTS incident_coordination_messages CASCADE;
DROP TABLE IF EXISTS incident_timeline_entries CASCADE;
DROP TABLE IF EXISTS incident_alerts CASCADE;
DROP TABLE IF EXISTS incident_documents CASCADE;
DROP TABLE IF EXISTS coordination_tasks CASCADE;
DROP TABLE IF EXISTS incidents CASCADE;
