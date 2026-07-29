package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type TriageRule struct {
	bun.BaseModel `bun:"table:triage_rules"`

	ID          uuid.UUID        `bun:"id,pk"`
	Name        string           `bun:"name,notnull"`
	Description string           `bun:"description,default:''"`
	Conditions  []map[string]any `bun:"conditions,type:jsonb"`
	MatchMode   string           `bun:"match_mode,notnull,default:'all'"`
	Decision    string           `bun:"decision,notnull"`
	Severity    *string          `bun:"severity"`
	Category    *string          `bun:"category"`
	Enrichment  map[string]any   `bun:"enrichment,type:jsonb"`
	Priority    int              `bun:"priority,notnull,default:0"`
	Enabled     bool             `bun:"enabled,notnull,default:true"`
	CreatedBy   *uuid.UUID       `bun:"created_by"`
	CreatedAt   time.Time        `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt   time.Time        `bun:"updated_at,notnull,default:current_timestamp"`
}
