-- +goose Up

-- audit_logs ----------------------------------------------------------------
-- Append-only security trail. `timestamp` is the single authoritative event
-- time (read in store/audit.go); the previously redundant created_at/updated_at
-- columns are dropped and there is no updated_at trigger. user_id / entity_id
-- are intentionally FK-free: audit writes are fire-and-forget and must succeed
-- even for deleted users, failed-auth identities, or polymorphic entities.
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    event TEXT NOT NULL CHECK (event <> ''),
    user_id UUID NULL,
    username TEXT NULL DEFAULT '',
    ip TEXT NULL DEFAULT '',
    user_agent TEXT NULL DEFAULT '',
    success BOOLEAN NOT NULL DEFAULT true,
    details JSONB NULL,
    request_id TEXT NULL DEFAULT '',
    entity_type TEXT NULL DEFAULT '',
    entity_id UUID NULL
);
CREATE INDEX audit_logs_user_id_timestamp ON audit_logs (user_id, timestamp);
CREATE INDEX audit_logs_event_timestamp ON audit_logs (event, timestamp);
CREATE INDEX audit_logs_entity_type_entity_id_timestamp ON audit_logs (entity_type, entity_id, timestamp);
CREATE INDEX audit_logs_request_id ON audit_logs (request_id);

-- outboxes ------------------------------------------------------------------
-- Transactional outbox. Rows advance pending -> published/failed; there is no
-- updated_at column (published_at / next_attempt_at track progress) so no
-- trigger is attached. Polymorphic aggregate_id stays TEXT (no FK).
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ NULL,
    next_attempt_at TIMESTAMPTZ NULL
);
CREATE INDEX outboxes_next_attempt_at_created_at ON outboxes (next_attempt_at, created_at) WHERE status IN ('pending', 'failed');
CREATE INDEX outboxes_aggregate_id ON outboxes (aggregate_id);
CREATE INDEX outboxes_event_id ON outboxes (event_id);

-- +goose Down
DROP TABLE IF EXISTS outboxes CASCADE;
DROP TABLE IF EXISTS audit_logs CASCADE;
