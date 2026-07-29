-- +goose Up
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event TEXT NOT NULL CHECK (event <> ''),
    user_id UUID NULL,
    username TEXT NULL DEFAULT '',
    ip TEXT NULL DEFAULT '',
    user_agent TEXT NULL DEFAULT '',
    success BOOLEAN NOT NULL DEFAULT true,
    details JSONB NULL,
    request_id TEXT NULL DEFAULT '',
    entity_type TEXT NULL DEFAULT '',
    entity_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_logs_user_id_timestamp ON audit_logs (user_id, timestamp);
CREATE INDEX audit_logs_event_timestamp ON audit_logs (event, timestamp);
CREATE INDEX audit_logs_entity_type_entity_id_timestamp ON audit_logs (entity_type, entity_id, timestamp);
CREATE INDEX audit_logs_request_id ON audit_logs (request_id);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
