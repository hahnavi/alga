package models

import (
	"time"

	"github.com/google/uuid"
)

type IncidentDocument struct {
	ID          uuid.UUID  `bun:"id,pk"`
	Section     string     `bun:"section,notnull,default:'current_status'"`
	Content     string     `bun:"content,notnull,default:''"`
	Version     int        `bun:"version,notnull,default:1"`
	CreatedAt   time.Time  `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt   time.Time  `bun:"updated_at,notnull,default:current_timestamp"`
	IncidentID  uuid.UUID  `bun:"incident_id,notnull"`
	UpdatedByID *uuid.UUID `bun:"updated_by_id"`
}
