package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// SystemConfig is a singleton configuration row.
// It does not embed BaseModel: the table name is singular (system_config)
// and Bun only honors `table:` on a direct bun.BaseModel field, so the
// shared BaseModel pattern would resolve to the pluralized system_configs
// and fail with "relation does not exist".
type SystemConfig struct {
	bun.BaseModel `bun:"table:system_config"`

	ID        uuid.UUID `bun:"id,pk"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp"`

	Config map[string]any `bun:"config,type:jsonb"`
}
