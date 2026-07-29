package models

import (
	"time"

	"github.com/google/uuid"
)

type AlertInvestigationAlert struct {
	BaseModel

	AlertInvestigationID uuid.UUID         `bun:"alert_investigation_id,notnull"`
	AlertID              *uuid.UUID        `bun:"alert_id"`
	Fingerprint          string            `bun:"fingerprint,notnull"`
	AlertNumber          int64             `bun:"alert_number"`
	Status               string            `bun:"status"`
	Alertname            string            `bun:"alertname"`
	Namespace            string            `bun:"namespace"`
	Labels               map[string]string `bun:"labels,type:jsonb"`
	Annotations          map[string]string `bun:"annotations,type:jsonb"`
	StartsAt             *time.Time        `bun:"starts_at"`
	EndsAt               *time.Time        `bun:"ends_at"`
	GeneratorURL         string            `bun:"generator_url"`
	Summary              string            `bun:"summary"`
	Current              bool              `bun:"current,notnull,default:true"`
}

func (*AlertInvestigationAlert) TableName() string { return "alert_investigation_alerts" }
