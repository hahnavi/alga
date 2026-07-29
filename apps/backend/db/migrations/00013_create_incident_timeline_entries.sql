-- +goose Up
CREATE TABLE incident_timeline_entries (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL DEFAULT 'note_added',
    actor_id UUID NULL,
    actor_type TEXT NOT NULL DEFAULT 'system',
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ics_event_type TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE
);

CREATE INDEX incident_timeline_entries_incident_id_created_at ON incident_timeline_entries (incident_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS incident_timeline_entries;
