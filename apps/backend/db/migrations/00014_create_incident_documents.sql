-- +goose Up
CREATE TABLE incident_documents (
    id UUID PRIMARY KEY,
    section TEXT NOT NULL DEFAULT 'current_status',
    content TEXT NOT NULL DEFAULT '',
    version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    updated_by_id UUID NULL REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX incident_documents_incident_id_section ON incident_documents (incident_id, section);
CREATE INDEX incident_documents_updated_by_id ON incident_documents (updated_by_id);

-- +goose Down
DROP TABLE IF EXISTS incident_documents;
