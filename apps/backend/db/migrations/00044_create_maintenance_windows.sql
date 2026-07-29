-- +goose Up
CREATE TABLE maintenance_windows (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    label_matchers JSONB NULL,
    created_by TEXT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX maintenance_windows_enabled_start_time_end_time ON maintenance_windows (enabled, start_time, end_time);

-- +goose Down
DROP TABLE IF EXISTS maintenance_windows;
