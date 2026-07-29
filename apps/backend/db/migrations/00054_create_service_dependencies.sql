-- +goose Up
CREATE TABLE service_dependencies (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL,
    dependent_on_service_id UUID NOT NULL,
    dependency_type TEXT NOT NULL DEFAULT 'depends_on' CHECK (dependency_type IN ('depends_on', 'hard', 'soft')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT service_dependencies_service_id_fk FOREIGN KEY (service_id) REFERENCES services (id) ON DELETE CASCADE,
    CONSTRAINT service_dependencies_dependent_on_service_id_fk FOREIGN KEY (dependent_on_service_id) REFERENCES services (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX service_dependencies_service_id_dependent_on_service_id ON service_dependencies (service_id, dependent_on_service_id);
CREATE INDEX service_dependencies_dependent_on_service_id ON service_dependencies (dependent_on_service_id);

-- +goose Down
DROP TABLE IF EXISTS service_dependencies;
