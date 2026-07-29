-- +goose Up
CREATE TABLE investigation_threads (
    id UUID PRIMARY KEY,
    thread_id TEXT NOT NULL UNIQUE,
    owner_type TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_investigation_threads_owner ON investigation_threads (owner_type, owner_id);

-- +goose Down
DROP TABLE IF EXISTS investigation_threads;
