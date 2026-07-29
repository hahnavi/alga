-- +goose Up
CREATE TABLE agent_dm_messages (
    id UUID PRIMARY KEY,
    chat_id TEXT NOT NULL DEFAULT 'alga_dm',
    role TEXT NOT NULL CHECK (role IN ('user', 'agent')),
    body TEXT NOT NULL,
    user_id TEXT,
    username TEXT,
    edited BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    agent_token_id UUID NOT NULL,
    CONSTRAINT fk_agent_dm_messages_agent_token FOREIGN KEY (agent_token_id) REFERENCES agent_tokens(id) ON DELETE CASCADE
);

CREATE INDEX idx_agent_dm_messages_chat_id_created_at ON agent_dm_messages (chat_id, created_at);
CREATE INDEX idx_agent_dm_messages_agent_token_id ON agent_dm_messages (agent_token_id);

-- +goose Down
DROP TABLE IF EXISTS agent_dm_messages;
