package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type TriageResult struct {
	bun.BaseModel `bun:"table:triage_results"`

	ID                 uuid.UUID         `bun:"id,pk"`
	TriageNumber       int64             `bun:"triage_number,notnull,unique"`
	CorrelationKey     string            `bun:"correlation_key,notnull"`
	AlertCount         int               `bun:"alert_count,notnull,default:0"`
	AlertFingerprints  []string          `bun:"alert_fingerprints,type:jsonb"`
	AlertLabels        map[string]string `bun:"alert_labels,type:jsonb"`
	SeverityInput      *string           `bun:"severity_input"`
	Decision           string            `bun:"decision,notnull"`
	Confidence         float64           `bun:"confidence,notnull,default:0"`
	SeverityClassified *string           `bun:"severity_classified"`
	Category           *string           `bun:"category"`
	Reasoning          string            `bun:"reasoning,default:''"`
	SuggestedActions   []string          `bun:"suggested_actions,type:jsonb"`
	Enrichment         map[string]any    `bun:"enrichment,type:jsonb"`
	ContextUsed        map[string]any    `bun:"context_used,type:jsonb"`
	Outcome            string            `bun:"outcome,notnull,default:'pending'"`
	OverriddenTo       *string           `bun:"overridden_to"`
	OverriddenBy       *uuid.UUID        `bun:"overridden_by"`
	OverriddenAt       *time.Time        `bun:"overridden_at"`
	ModelUsed          string            `bun:"model_used,default:''"`
	TriageDurationMs   int64             `bun:"triage_duration_ms,notnull,default:0"`
	TraceID            string            `bun:"trace_id,default:''"`
	CreatedAt          time.Time         `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt          time.Time         `bun:"updated_at,notnull,default:current_timestamp"`
}
