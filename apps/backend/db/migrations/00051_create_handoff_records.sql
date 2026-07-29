-- +goose Up
CREATE TABLE handoff_records (
    id UUID PRIMARY KEY,
    schedule_id UUID NOT NULL,
    outgoing_user_id UUID NULL,
    incoming_user_id UUID NULL,
    handoff_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'acknowledged')),
    outgoing_notes TEXT NULL,
    incoming_notes TEXT NULL,
    incoming_acknowledged_at TIMESTAMPTZ NULL,
    incident_summary TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT handoff_records_schedule_id_fk FOREIGN KEY (schedule_id) REFERENCES on_call_schedules (id) ON DELETE CASCADE,
    CONSTRAINT handoff_records_outgoing_user_id_fk FOREIGN KEY (outgoing_user_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT handoff_records_incoming_user_id_fk FOREIGN KEY (incoming_user_id) REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX handoff_records_schedule_id_handoff_at ON handoff_records (schedule_id, handoff_at);
CREATE INDEX handoff_records_incoming_user_id_status ON handoff_records (incoming_user_id, status);
CREATE INDEX handoff_records_outgoing_user_id ON handoff_records (outgoing_user_id);
CREATE INDEX handoff_records_status_handoff_at ON handoff_records (status, handoff_at);

-- +goose Down
DROP TABLE IF EXISTS handoff_records;
