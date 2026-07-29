package models

import (
	"github.com/google/uuid"
)

type AlertInvestigationEvent struct {
	BaseModel

	AlertInvestigationID uuid.UUID      `bun:"alert_investigation_id,notnull"`
	EventType            string         `bun:"event_type,notnull"`
	Reason               string         `bun:"reason"`
	ActorType            string         `bun:"actor_type,default:'system'"`
	ActorID              string         `bun:"actor_id"`
	ActorName            string         `bun:"actor_name"`
	AgentID              string         `bun:"agent_id"`
	AgentName            string         `bun:"agent_name"`
	AgentType            string         `bun:"agent_type"`
	Metadata             map[string]any `bun:"metadata,type:jsonb"`
}

func (*AlertInvestigationEvent) TableName() string { return "alert_investigation_events" }
