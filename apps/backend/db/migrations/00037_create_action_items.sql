-- +goose Up
CREATE TABLE action_items (
    id UUID PRIMARY KEY,
    post_mortem_id UUID NOT NULL,
    description TEXT NOT NULL CHECK (description <> ''),
    type TEXT NOT NULL DEFAULT 'investigate' CHECK (type IN ('prevent', 'mitigate', 'detect', 'investigate')),
    assignee_name TEXT NULL DEFAULT '',
    assignee_id UUID NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'detected', 'in_progress', 'completed', 'cancelled')),
    priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high')),
    due_date TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT action_items_post_mortem_id_fk FOREIGN KEY (post_mortem_id) REFERENCES post_mortems (id) ON DELETE CASCADE
);

CREATE INDEX action_items_post_mortem_id ON action_items (post_mortem_id);
CREATE INDEX action_items_assignee_id_status ON action_items (assignee_id, status);
CREATE INDEX action_items_status ON action_items (status);
CREATE INDEX action_items_type ON action_items (type);

-- +goose Down
DROP TABLE IF EXISTS action_items;
