package models

import (
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID                     uuid.UUID  `bun:"id,pk"`
	UserID                 uuid.UUID  `bun:"user_id,notnull"`
	Type                   string     `bun:"type,notnull"`
	Title                  string     `bun:"title,notnull"`
	Message                string     `bun:"message,notnull"`
	Read                   bool       `bun:"read,notnull,default:false"`
	ResourceType           *string    `bun:"resource_type"`
	ResourceID             string     `bun:"resource_id,default:''"`
	TriggeredByUserID      *uuid.UUID `bun:"triggered_by_user_id"`
	TriggeredByDisplayName string     `bun:"triggered_by_display_name,default:''"`
	CreatedAt              time.Time  `bun:"created_at,notnull,default:current_timestamp"`
}
