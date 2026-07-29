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
	UpdatedAt   time.Time  `bun:"updated_at,notnull,default:current_timestamp"`
	IncidentID  uuid.UUID  `bun:"incident_id,notnull"`
	UpdatedByID *uuid.UUID `bun:"updated_by_id"`
}

func (*IncidentDocument) TableName() string { return "incident_documents" }
