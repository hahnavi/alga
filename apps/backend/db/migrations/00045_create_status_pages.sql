-- +goose Up
CREATE TABLE status_pages (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    slug TEXT NOT NULL UNIQUE CHECK (slug <> ''),
    description TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'internal' CHECK (visibility IN ('internal', 'public')),
    enabled BOOLEAN NOT NULL DEFAULT true,
    owner_team_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT status_pages_owner_team_id_fk FOREIGN KEY (owner_team_id) REFERENCES teams (id) ON DELETE SET NULL
);

CREATE INDEX status_pages_enabled ON status_pages (enabled);
CREATE INDEX status_pages_owner_team_id ON status_pages (owner_team_id);

-- +goose Down
DROP TABLE IF EXISTS status_pages;
