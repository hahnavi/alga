package models

import (
	"github.com/google/uuid"
)

type PlaybookStep struct {
	BaseModel `bun:"table:playbook_steps"`

	PlaybookID       uuid.UUID `bun:"playbook_id,notnull"`
	StepNumber       int       `bun:"step_number,notnull"`
	Title            string    `bun:"title,notnull"`
	Description      string    `bun:"description"`
	ExpectedDuration string    `bun:"expected_duration"`
	Command          string    `bun:"command"`
}
