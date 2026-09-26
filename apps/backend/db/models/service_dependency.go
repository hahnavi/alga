package models

import (
	"github.com/google/uuid"
)

type ServiceDependency struct {
	CreatedOnlyModel `bun:"table:service_dependencies"`

	ServiceID            uuid.UUID `bun:"service_id,notnull"`
	DependentOnServiceID uuid.UUID `bun:"dependent_on_service_id,notnull"`
	DependencyType       string    `bun:"dependency_type,notnull,default:'depends_on'"`
}
