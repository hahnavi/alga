package models

import (
	"github.com/google/uuid"
)

type Playbook struct {
	BaseModel `bun:"table:playbooks"`

	Title          string           `bun:"title,notnull,unique"`
	Kind           string           `bun:"kind,notnull"`
	Summary        string           `bun:"summary"`
	ServiceID      *uuid.UUID       `bun:"service_id"`
	LabelSelectors []map[string]any `bun:"label_selectors,type:jsonb"`
	Tags           []string         `bun:"tags,type:jsonb"`
	CreatedBy      uuid.UUID        `bun:"created_by,notnull"`
}
