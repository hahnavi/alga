package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Playbook struct {
	bun.BaseModel `bun:"table:playbooks"`

	ID             uuid.UUID        `bun:"id,pk"`
	Title          string           `bun:"title,notnull,unique"`
	Kind           string           `bun:"kind,notnull"`
	Summary        string           `bun:"summary"`
	ServiceID      *uuid.UUID       `bun:"service_id"`
	LabelSelectors []map[string]any `bun:"label_selectors,type:jsonb"`
	Tags           []string         `bun:"tags,type:jsonb"`
	CreatedBy      uuid.UUID        `bun:"created_by,notnull"`
	CreatedAt      time.Time        `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt      time.Time        `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*Playbook) TableName() string { return "playbooks" }
