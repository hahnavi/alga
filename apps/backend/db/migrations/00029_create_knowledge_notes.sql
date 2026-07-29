-- +goose Up
CREATE TABLE knowledge_notes (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('runbook', 'known_issue', 'service_owner', 'fact')),
    title TEXT NOT NULL,
    body_markdown TEXT NOT NULL,
    tags JSONB,
    selectors JSONB,
    author_id UUID,
    author_type TEXT NOT NULL DEFAULT 'user' CHECK (author_type IN ('user', 'agent')),
    author_name TEXT DEFAULT '',
    source_investigation_id TEXT DEFAULT '',
    confidence DOUBLE PRECISION,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_knowledge_notes_author FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_knowledge_notes_kind_updated_at ON knowledge_notes (kind, updated_at);
CREATE INDEX idx_knowledge_notes_expires_at ON knowledge_notes (expires_at);
CREATE INDEX idx_knowledge_notes_author_id ON knowledge_notes (author_id);
CREATE INDEX idx_knowledge_notes_fts ON knowledge_notes USING gin(to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body_markdown, '') || ' ' || coalesce(tags::text, '')));
CREATE INDEX idx_knowledge_notes_tags ON knowledge_notes USING gin(tags jsonb_path_ops);

-- +goose Down
DROP TABLE IF EXISTS knowledge_notes;
