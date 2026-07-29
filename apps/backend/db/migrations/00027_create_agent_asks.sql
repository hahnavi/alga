-- +goose Up
CREATE TABLE agent_asks (
    id UUID PRIMARY KEY,
    from_agent_id UUID NOT NULL,
    from_agent_name TEXT NOT NULL,
    from_agent_type TEXT NOT NULL DEFAULT 'hermes' CHECK (from_agent_type IN ('hermes', 'openclaw', 'other')),
    investigation_id TEXT DEFAULT '',
    to_agent_id UUID,
    to_agent_type TEXT CHECK (to_agent_type IN ('hermes', 'openclaw', 'other')),
    question TEXT NOT NULL,
    reply TEXT DEFAULT '',
    replied_by_agent_id UUID,
    replied_by_agent_name TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'answered', 'expired', 'cancelled')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at TIMESTAMPTZ,
    CONSTRAINT fk_agent_asks_from_agent FOREIGN KEY (from_agent_id) REFERENCES agent_tokens(id) ON DELETE RESTRICT,
    CONSTRAINT fk_agent_asks_to_agent FOREIGN KEY (to_agent_id) REFERENCES agent_tokens(id) ON DELETE SET NULL,
    CONSTRAINT fk_agent_asks_replied_by_agent FOREIGN KEY (replied_by_agent_id) REFERENCES agent_tokens(id) ON DELETE SET NULL
);

CREATE INDEX idx_agent_asks_status_created_at ON agent_asks (status, created_at);
CREATE INDEX idx_agent_asks_to_agent_id_status ON agent_asks (to_agent_id, status);
CREATE INDEX idx_agent_asks_to_agent_type_status ON agent_asks (to_agent_type, status);
CREATE INDEX idx_agent_asks_from_agent_id_created_at ON agent_asks (from_agent_id, created_at);
CREATE INDEX idx_agent_asks_investigation_id ON agent_asks (investigation_id);
CREATE INDEX idx_agent_asks_expires_at ON agent_asks (expires_at);
CREATE INDEX idx_agent_asks_replied_by_agent_id ON agent_asks (replied_by_agent_id);

-- +goose Down
DROP TABLE IF EXISTS agent_asks;
