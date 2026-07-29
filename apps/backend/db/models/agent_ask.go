package models

import (
	"time"

	"github.com/google/uuid"
)

type AgentAsk struct {
	BaseModel

	FromAgentID        uuid.UUID  `bun:"from_agent_id,notnull"`
	FromAgentName      string     `bun:"from_agent_name,notnull"`
	FromAgentType      string     `bun:"from_agent_type,notnull,default:'hermes'"`
	InvestigationID    string     `bun:"investigation_id"`
	ToAgentID          *uuid.UUID `bun:"to_agent_id"`
	ToAgentType        *string    `bun:"to_agent_type"`
	Question           string     `bun:"question,notnull"`
	Reply              string     `bun:"reply"`
	RepliedByAgentID   *uuid.UUID `bun:"replied_by_agent_id"`
	RepliedByAgentName string     `bun:"replied_by_agent_name"`
	Status             string     `bun:"status,notnull,default:'pending'"`
	ExpiresAt          time.Time  `bun:"expires_at,notnull"`
	AnsweredAt         *time.Time `bun:"answered_at"`
}
