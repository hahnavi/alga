package models

import (
	"time"

	"github.com/google/uuid"
)

type Alert struct {
	BaseModel
	SoftDeleteModel

	Fingerprint        string            `bun:"fingerprint,notnull"`
	Status             string            `bun:"status,notnull,default:'firing'"`
	Acknowledged       bool              `bun:"acknowledged,notnull,default:false"`
	Silenced           bool              `bun:"silenced,notnull,default:false"`
	Labels             map[string]string `bun:"labels,type:jsonb,notnull,default:'{}'"`
	Annotations        map[string]string `bun:"annotations,type:jsonb,notnull,default:'{}'"`
	Values             map[string]any    `bun:"values,type:jsonb"`
	StartsAt           time.Time         `bun:"starts_at,notnull,default:current_timestamp"`
	EndsAt             *time.Time        `bun:"ends_at"`
	GeneratorURL       string            `bun:"generator_url"`
	AlertNumber        int64             `bun:"alert_number,notnull,unique"`
	TriageResultID     *uuid.UUID        `bun:"triage_result_id"`
	Enrichment         map[string]any    `bun:"enrichment,type:jsonb"`
	TriageCategory     string            `bun:"triage_category"`
	SeverityClassified string            `bun:"severity_classified"`
}
