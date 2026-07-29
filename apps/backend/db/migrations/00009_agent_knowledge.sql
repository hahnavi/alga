-- +goose Up

-- agent_memories ------------------------------------------------------------
-- Semantic memory store. The redundant `embedding JSONB` column is dropped;
-- `vec vector(1536)` is the single source of truth for embeddings (HNSW cosine).
-- The FTS expression to_tsvector('english', content) must stay byte-identical
-- to store/agent_memory.go. agent_id is a real FK to agent_tokens.
CREATE TABLE agent_memories (
    id UUID PRIMARY KEY,
    content TEXT NOT NULL,
    memory_type TEXT NOT NULL DEFAULT 'fact' CHECK (memory_type IN ('fact', 'pattern', 'procedure')),
    hash TEXT NOT NULL UNIQUE,
    vec vector(1536),
    agent_id UUID,
    agent_name TEXT NOT NULL DEFAULT '',
    agent_type TEXT NOT NULL DEFAULT '',
    investigation_id TEXT NOT NULL DEFAULT '',
    correlation_key TEXT NOT NULL DEFAULT '',
    labels JSONB,
    entities JSONB,
    metadata JSONB,
    confidence DOUBLE PRECISION CHECK (confidence >= 0 AND confidence <= 1),
    access_count INT NOT NULL DEFAULT 0 CHECK (access_count >= 0),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_memories_agent_id_fk FOREIGN KEY (agent_id) REFERENCES agent_tokens (id) ON DELETE SET NULL
);
CREATE INDEX agent_memories_agent_id_created_at ON agent_memories (agent_id, created_at);
CREATE INDEX agent_memories_investigation_id ON agent_memories (investigation_id);
CREATE INDEX agent_memories_memory_type ON agent_memories (memory_type);
CREATE INDEX agent_memories_expires_at ON agent_memories (expires_at);
CREATE INDEX agent_memories_correlation_key ON agent_memories (correlation_key);
CREATE INDEX agent_memories_vec ON agent_memories USING hnsw (vec vector_cosine_ops);
CREATE INDEX agent_memories_fts ON agent_memories USING gin(to_tsvector('english', content));
CREATE TRIGGER trg_agent_memories_set_updated_at BEFORE UPDATE ON agent_memories FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- agent_asks ----------------------------------------------------------------
CREATE TABLE agent_asks (
    id UUID PRIMARY KEY,
    from_agent_id UUID NOT NULL,
    from_agent_name TEXT NOT NULL,
    from_agent_type TEXT NOT NULL DEFAULT 'hermes' CHECK (from_agent_type IN ('hermes', 'openclaw', 'other')),
    investigation_id TEXT NOT NULL DEFAULT '',
    to_agent_id UUID,
    to_agent_type TEXT CHECK (to_agent_type IN ('hermes', 'openclaw', 'other')),
    question TEXT NOT NULL,
    reply TEXT NOT NULL DEFAULT '',
    replied_by_agent_id UUID,
    replied_by_agent_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'answered', 'expired', 'cancelled')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at TIMESTAMPTZ,
    CONSTRAINT agent_asks_from_agent_fk FOREIGN KEY (from_agent_id) REFERENCES agent_tokens (id) ON DELETE RESTRICT,
    CONSTRAINT agent_asks_to_agent_fk FOREIGN KEY (to_agent_id) REFERENCES agent_tokens (id) ON DELETE SET NULL,
    CONSTRAINT agent_asks_replied_by_agent_fk FOREIGN KEY (replied_by_agent_id) REFERENCES agent_tokens (id) ON DELETE SET NULL
);
CREATE INDEX agent_asks_status_created_at ON agent_asks (status, created_at);
CREATE INDEX agent_asks_to_agent_id_status ON agent_asks (to_agent_id, status);
CREATE INDEX agent_asks_to_agent_type_status ON agent_asks (to_agent_type, status);
CREATE INDEX agent_asks_from_agent_id_created_at ON agent_asks (from_agent_id, created_at);
CREATE INDEX agent_asks_investigation_id ON agent_asks (investigation_id);
CREATE INDEX agent_asks_expires_at ON agent_asks (expires_at);
CREATE INDEX agent_asks_replied_by_agent_id ON agent_asks (replied_by_agent_id);
CREATE TRIGGER trg_agent_asks_set_updated_at BEFORE UPDATE ON agent_asks FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- agent_dm_messages ---------------------------------------------------------
CREATE TABLE agent_dm_messages (
    id UUID PRIMARY KEY,
    chat_id TEXT NOT NULL DEFAULT 'alga_dm',
    role TEXT NOT NULL CHECK (role IN ('user', 'agent')),
    body TEXT NOT NULL,
    user_id TEXT,
    username TEXT,
    edited BOOLEAN NOT NULL DEFAULT false,
    agent_token_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_dm_messages_agent_token_fk FOREIGN KEY (agent_token_id) REFERENCES agent_tokens (id) ON DELETE CASCADE
);
CREATE INDEX agent_dm_messages_chat_id_created_at ON agent_dm_messages (chat_id, created_at);
CREATE INDEX agent_dm_messages_agent_token_id ON agent_dm_messages (agent_token_id);
CREATE TRIGGER trg_agent_dm_messages_set_updated_at BEFORE UPDATE ON agent_dm_messages FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- knowledge_notes -----------------------------------------------------------
-- The FTS expression MUST stay byte-identical to knowledgeFTSExpression in
-- store/knowledge.go (title + body_markdown + tags::text).
CREATE TABLE knowledge_notes (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('runbook', 'known_issue', 'service_owner', 'fact')),
    title TEXT NOT NULL,
    body_markdown TEXT NOT NULL,
    tags JSONB,
    selectors JSONB,
    author_id UUID,
    author_type TEXT NOT NULL DEFAULT 'user' CHECK (author_type IN ('user', 'agent')),
    author_name TEXT NOT NULL DEFAULT '',
    source_investigation_id TEXT NOT NULL DEFAULT '',
    confidence DOUBLE PRECISION,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT knowledge_notes_author_fk FOREIGN KEY (author_id) REFERENCES users (id) ON DELETE SET NULL
);
CREATE INDEX knowledge_notes_kind_updated_at ON knowledge_notes (kind, updated_at);
CREATE INDEX knowledge_notes_expires_at ON knowledge_notes (expires_at);
CREATE INDEX knowledge_notes_author_id ON knowledge_notes (author_id);
CREATE INDEX knowledge_notes_fts ON knowledge_notes USING gin(to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body_markdown, '') || ' ' || coalesce(tags::text, '')));
CREATE INDEX knowledge_notes_tags ON knowledge_notes USING gin(tags jsonb_path_ops);
CREATE TRIGGER trg_knowledge_notes_set_updated_at BEFORE UPDATE ON knowledge_notes FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS knowledge_notes CASCADE;
DROP TABLE IF EXISTS agent_dm_messages CASCADE;
DROP TABLE IF EXISTS agent_asks CASCADE;
DROP TABLE IF EXISTS agent_memories CASCADE;
