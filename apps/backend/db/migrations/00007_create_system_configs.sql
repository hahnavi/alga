-- +goose Up
CREATE TABLE system_config (
    id UUID PRIMARY KEY,
    config JSONB NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS system_config;
