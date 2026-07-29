package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// SystemConfig is a singleton configuration row.
type SystemConfig struct {
	bun.BaseModel `bun:"table:system_config"`

	ID        uuid.UUID      `bun:"id,pk"`
	Config    map[string]any `bun:"config,type:jsonb"`
	CreatedAt time.Time      `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt time.Time      `bun:"updated_at,notnull,default:current_timestamp"`
}
