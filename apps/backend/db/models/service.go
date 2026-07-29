package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Service struct {
	bun.BaseModel `bun:"table:services"`

	ID                 uuid.UUID        `bun:"id,pk"`
	Name               string           `bun:"name,notnull,unique"`
	DisplayName        string           `bun:"display_name,notnull,default:''"`
	Description        string           `bun:"description,notnull,default:''"`
	OwnerTeamID        *uuid.UUID       `bun:"owner_team_id"`
	EscalationPolicyID *uuid.UUID       `bun:"escalation_policy_id"`
	LabelMatchers      []map[string]any `bun:"label_matchers,type:jsonb,notnull,default:'[]'"`
	SLAResponseMinutes int              `bun:"sla_response_minutes,notnull,default:0"`
	SLAResolveMinutes  int              `bun:"sla_resolve_minutes,notnull,default:0"`
	Status             string           `bun:"status,notnull,default:'operational'"`
	CreatedAt          time.Time        `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt          time.Time        `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*Service) TableName() string { return "services" }
