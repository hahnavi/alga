-- +goose Up
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    incident_id UUID NOT NULL,
    CONSTRAINT incident_coordination_messages_incident_id_fk FOREIGN KEY (incident_id) REFERENCES incidents (id) ON DELETE CASCADE,
    CONSTRAINT incident_coordination_messages_parent_message_id_fk FOREIGN KEY (parent_message_id) REFERENCES incident_coordination_messages (id) ON DELETE SET NULL,
    CONSTRAINT incident_coordination_messages_linked_task_id_fk FOREIGN KEY (linked_coordination_task_id) REFERENCES coordination_tasks (id) ON DELETE SET NULL
);

CREATE INDEX incident_coordination_messages_created_at ON incident_coordination_messages (created_at);
CREATE INDEX incident_coordination_messages_provider_message_id ON incident_coordination_messages (provider_message_id);
CREATE INDEX incident_coordination_messages_slack_channel_id_slack_message_ts ON incident_coordination_messages (slack_channel_id, slack_message_ts);
CREATE INDEX incident_coordination_messages_parent_message_id ON incident_coordination_messages (parent_message_id);
CREATE INDEX incident_coordination_messages_linked_coordination_task_id ON incident_coordination_messages (linked_coordination_task_id);
CREATE INDEX incident_coordination_messages_incident_id_created_at ON incident_coordination_messages (incident_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS incident_coordination_messages;
