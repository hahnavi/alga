package models

import (
	"github.com/google/uuid"
)

type TriageRule struct {
	BaseModel `bun:"table:triage_rules"`

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
}
