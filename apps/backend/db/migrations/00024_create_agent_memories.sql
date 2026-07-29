-- +goose Up
CREATE TABLE agent_memories (
    id UUID PRIMARY KEY,
    content TEXT NOT NULL,
    memory_type TEXT NOT NULL DEFAULT 'fact' CHECK (memory_type IN ('fact', 'pattern', 'procedure')),
    hash TEXT NOT NULL UNIQUE,
    embedding JSONB,
    vec vector(1536),
    agent_id UUID,
    agent_name TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    investigation_id TEXT DEFAULT '',
    correlation_key TEXT DEFAULT '',
    labels JSONB,
    entities JSONB,
    metadata JSONB,
    confidence DOUBLE PRECISION CHECK (confidence >= 0 AND confidence <= 1),
    access_count INT NOT NULL DEFAULT 0 CHECK (access_count >= 0),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_memories_agent_id_created_at ON agent_memories (agent_id, created_at);
CREATE INDEX idx_agent_memories_investigation_id ON agent_memories (investigation_id);
CREATE INDEX idx_agent_memories_memory_type ON agent_memories (memory_type);
CREATE INDEX idx_agent_memories_expires_at ON agent_memories (expires_at);
CREATE INDEX idx_agent_memories_correlation_key ON agent_memories (correlation_key);
CREATE INDEX idx_agent_memories_vec ON agent_memories USING hnsw (vec vector_cosine_ops);
CREATE INDEX idx_agent_memories_fts ON agent_memories USING gin(to_tsvector('english', content));

-- +goose Down
DROP TABLE IF EXISTS agent_memories;
