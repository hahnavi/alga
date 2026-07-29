-- +goose Up
CREATE TABLE route_rules (
    id UUID PRIMARY KEY,
    routes JSONB NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS route_rules;
