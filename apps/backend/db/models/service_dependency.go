package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ServiceDependency struct {
	bun.BaseModel `bun:"table:service_dependencies"`

	ID                   uuid.UUID `bun:"id,pk"`
	ServiceID            uuid.UUID `bun:"service_id,notnull"`
	DependentOnServiceID uuid.UUID `bun:"dependent_on_service_id,notnull"`
	DependencyType       string    `bun:"dependency_type,notnull,default:'depends_on'"`
	CreatedAt            time.Time `bun:"created_at,notnull,default:current_timestamp"`
}
