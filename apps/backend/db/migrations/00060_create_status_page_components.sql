-- +goose Up
CREATE TABLE status_page_components (
    id UUID PRIMARY KEY,
    status_page_id UUID NOT NULL,
    name TEXT NOT NULL CHECK (name <> ''),
    description TEXT NOT NULL DEFAULT '',
    service_id UUID NULL,
    display_order INT NOT NULL DEFAULT 0 CHECK (display_order >= 0),
    status TEXT NOT NULL DEFAULT 'operational' CHECK (status IN ('operational', 'degraded', 'partial_outage', 'major_outage', 'maintenance')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT status_page_components_status_page_id_fk FOREIGN KEY (status_page_id) REFERENCES status_pages (id) ON DELETE CASCADE,
    CONSTRAINT status_page_components_service_id_fk FOREIGN KEY (service_id) REFERENCES services (id) ON DELETE SET NULL
);

CREATE INDEX status_page_components_status_page_id_display_order ON status_page_components (status_page_id, display_order);
CREATE INDEX status_page_components_service_id ON status_page_components (service_id);

-- +goose Down
DROP TABLE IF EXISTS status_page_components;
