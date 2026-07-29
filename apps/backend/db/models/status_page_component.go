package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type StatusPageComponent struct {
	bun.BaseModel `bun:"table:status_page_components"`

	ID           uuid.UUID  `bun:"id,pk"`
	StatusPageID uuid.UUID  `bun:"status_page_id,notnull"`
	Name         string     `bun:"name,notnull"`
	Description  string     `bun:"description,notnull,default:''"`
	ServiceID    *uuid.UUID `bun:"service_id"`
	DisplayOrder int        `bun:"display_order,notnull,default:0"`
	Status       string     `bun:"status,notnull,default:'operational'"`
	CreatedAt    time.Time  `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt    time.Time  `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*StatusPageComponent) TableName() string { return "status_page_components" }
