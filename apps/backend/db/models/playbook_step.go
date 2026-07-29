package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type PlaybookStep struct {
	bun.BaseModel `bun:"table:playbook_steps"`

	ID               uuid.UUID `bun:"id,pk"`
	PlaybookID       uuid.UUID `bun:"playbook_id,notnull"`
	StepNumber       int       `bun:"step_number,notnull"`
	Title            string    `bun:"title,notnull"`
	Description      string    `bun:"description"`
	ExpectedDuration string    `bun:"expected_duration"`
	Command          string    `bun:"command"`
	CreatedAt        time.Time `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt        time.Time `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*PlaybookStep) TableName() string { return "playbook_steps" }
