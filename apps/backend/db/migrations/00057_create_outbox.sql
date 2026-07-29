-- +goose Up
CREATE TABLE outboxes (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL CHECK (event_type <> ''),
    aggregate_id TEXT NOT NULL DEFAULT '',
    exchange TEXT NOT NULL CHECK (exchange <> ''),
    routing_key TEXT NOT NULL DEFAULT '',
    payload BYTEA NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'published', 'failed')),
    event_id TEXT NOT NULL DEFAULT '',
    retry_count INT NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ NULL,
    next_attempt_at TIMESTAMPTZ NULL
);

CREATE INDEX outboxes_next_attempt_at_created_at ON outboxes (next_attempt_at, created_at) WHERE status IN ('pending', 'failed');
CREATE INDEX outboxes_aggregate_id ON outboxes (aggregate_id);
CREATE INDEX outboxes_event_id ON outboxes (event_id);

-- +goose Down
DROP TABLE IF EXISTS outboxes;
