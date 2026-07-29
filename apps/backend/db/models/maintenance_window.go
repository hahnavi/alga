package models

import (
	"time"
)

type MaintenanceWindow struct {
	BaseModel

	Name          string            `bun:"name,notnull"`
	StartTime     time.Time         `bun:"start_time,notnull"`
	EndTime       time.Time         `bun:"end_time,notnull"`
	LabelMatchers map[string]string `bun:"label_matchers,type:jsonb"`
	CreatedBy     string            `bun:"created_by"`
	Enabled       bool              `bun:"enabled,notnull,default:true"`
}
