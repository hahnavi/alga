-- +goose Up
CREATE TABLE playbooks (
    id UUID PRIMARY KEY,
    title TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('procedure', 'mitigation')),
    summary TEXT NULL,
    service_id UUID NULL,
    label_selectors JSONB NULL,
    tags JSONB NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT playbooks_service_id_fk FOREIGN KEY (service_id) REFERENCES services (id) ON DELETE SET NULL,
    CONSTRAINT playbooks_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS playbooks;
