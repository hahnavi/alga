package models

import (
	"github.com/google/uuid"
)

type StatusPageComponent struct {
	BaseModel `bun:"table:status_page_components"`

	StatusPageID uuid.UUID  `bun:"status_page_id,notnull"`
	Name         string     `bun:"name,notnull"`
	Description  string     `bun:"description,notnull,default:''"`
	ServiceID    *uuid.UUID `bun:"service_id"`
	DisplayOrder int        `bun:"display_order,notnull,default:0"`
	Status       string     `bun:"status,notnull,default:'operational'"`
}
