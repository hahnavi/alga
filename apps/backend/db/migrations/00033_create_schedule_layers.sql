-- +goose Up
CREATE TABLE schedule_layers (
    id UUID PRIMARY KEY,
    schedule_id UUID NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    rotation_type TEXT NOT NULL DEFAULT 'weekly' CHECK (rotation_type IN ('daily', 'weekly', 'custom')),
    rotation_interval INT NOT NULL DEFAULT 1 CHECK (rotation_interval > 0),
    start_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_date TIMESTAMPTZ NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    start_time TEXT NOT NULL DEFAULT '00:00',
    end_time TEXT NOT NULL DEFAULT '',
    days_of_week JSONB NOT NULL DEFAULT '[]',
    priority INT NOT NULL DEFAULT 0 CHECK (priority >= 0),
    user_ids JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT schedule_layers_schedule_id_fk FOREIGN KEY (schedule_id) REFERENCES on_call_schedules (id) ON DELETE CASCADE
);

CREATE INDEX schedule_layers_schedule_id ON schedule_layers (schedule_id);
CREATE UNIQUE INDEX schedule_layers_schedule_id_priority ON schedule_layers (schedule_id, priority);

-- +goose Down
DROP TABLE IF EXISTS schedule_layers;
