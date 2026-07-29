-- +goose Up
CREATE TABLE agent_tokens (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    agent_type TEXT NOT NULL DEFAULT 'hermes' CHECK (agent_type IN ('hermes', 'openclaw', 'other')),
    token_hash TEXT NOT NULL UNIQUE,
    lookup_prefix TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    revoked BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    scope TEXT DEFAULT '',
    label_selectors JSONB,
    default_for_investigation BOOLEAN DEFAULT false,
    capabilities JSONB DEFAULT '["investigate"]'
);

CREATE INDEX idx_agent_tokens_lookup_prefix ON agent_tokens (lookup_prefix) WHERE revoked = false AND enabled = true;
CREATE INDEX idx_agent_tokens_expires_at ON agent_tokens (expires_at);
CREATE INDEX idx_agent_tokens_default_for_investigation ON agent_tokens (default_for_investigation) WHERE default_for_investigation = true;

-- Add FK from agent_memories now that agent_tokens exists
ALTER TABLE agent_memories
    ADD CONSTRAINT fk_agent_memories_agent_id
    FOREIGN KEY (agent_id) REFERENCES agent_tokens(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE agent_memories DROP CONSTRAINT IF EXISTS fk_agent_memories_agent_id;
DROP TABLE IF EXISTS agent_tokens;
