-- +goose Up
CREATE TABLE post_mortems (
    id UUID PRIMARY KEY,
    incident_id UUID NOT NULL UNIQUE,
    title TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'approved', 'published')),
    summary TEXT NOT NULL DEFAULT '',
    timeline JSONB NULL,
    root_cause TEXT NOT NULL DEFAULT '',
    contributing_factors JSONB NULL,
    impact TEXT NOT NULL DEFAULT '',
    lessons_learned TEXT NOT NULL DEFAULT '',
    what_went_well TEXT NOT NULL DEFAULT '',
    what_went_wrong TEXT NOT NULL DEFAULT '',
    blameless_confirmed BOOLEAN NOT NULL DEFAULT false,
    blameless_notes TEXT NOT NULL DEFAULT '',
    approved_by_id UUID NULL,
    published_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT post_mortems_incident_id_fk FOREIGN KEY (incident_id) REFERENCES incidents (id) ON DELETE CASCADE,
    CONSTRAINT post_mortems_approved_by_id_fk FOREIGN KEY (approved_by_id) REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX post_mortems_incident_id ON post_mortems (incident_id);
CREATE INDEX post_mortems_status_created_at ON post_mortems (status, created_at);
CREATE INDEX post_mortems_approved_by_id ON post_mortems (approved_by_id);

-- +goose Down
DROP TABLE IF EXISTS post_mortems;
