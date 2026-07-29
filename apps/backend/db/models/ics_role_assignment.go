package models

import (
	"time"

	"github.com/google/uuid"
)

type ICSRoleAssignment struct {
	ID               uuid.UUID  `bun:"id,pk"`
	ParentID         *uuid.UUID `bun:"parent_id"`
	IncidentID       uuid.UUID  `bun:"incident_id,notnull"`
	UserID           *uuid.UUID `bun:"user_id"`
	AgentTokenID     *uuid.UUID `bun:"agent_token_id"`
	RoleType         string     `bun:"role_type,notnull,default:'responder'"`
	Status           string     `bun:"status,notnull,default:'active'"`
	AssigneeType     string     `bun:"assignee_type,notnull,default:'user'"`
	ScopeDescription *string    `bun:"scope_description"`
	EndedReason      *string    `bun:"ended_reason"`
	StartedAt        time.Time  `bun:"started_at,notnull,default:current_timestamp"`
	EndedAt          *time.Time `bun:"ended_at"`
}
