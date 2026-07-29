-- +goose Up
CREATE TABLE heartbeats (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    description TEXT NOT NULL DEFAULT '',
    interval_seconds INT NOT NULL CHECK (interval_seconds > 0),
    grace_seconds INT NOT NULL DEFAULT 60 CHECK (grace_seconds >= 0),
    enabled BOOLEAN NOT NULL DEFAULT true,
    owner_team_id UUID NULL,
    status TEXT NOT NULL DEFAULT 'healthy' CHECK (status IN ('healthy', 'expired')),
    severity TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('critical', 'warning', 'info')),
    labels JSONB NULL,
    ping_token_hash TEXT NOT NULL UNIQUE CHECK (ping_token_hash <> ''),
    lookup_prefix TEXT NOT NULL CHECK (lookup_prefix <> ''),
    last_ping_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NULL,
    last_breach_at TIMESTAMPTZ NULL,
    created_by TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT heartbeats_owner_team_id_fk FOREIGN KEY (owner_team_id) REFERENCES teams (id) ON DELETE SET NULL
);

CREATE INDEX heartbeats_enabled_status_expires_at ON heartbeats (enabled, status, expires_at);
CREATE INDEX heartbeats_enabled_last_ping_at ON heartbeats (enabled, last_ping_at);
CREATE INDEX heartbeats_owner_team_id ON heartbeats (owner_team_id);
CREATE INDEX heartbeats_lookup_prefix ON heartbeats (lookup_prefix);

-- +goose Down
DROP TABLE IF EXISTS heartbeats;
