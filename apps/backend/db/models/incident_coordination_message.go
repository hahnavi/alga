package models

import (
	"github.com/google/uuid"
)

type IncidentCoordinationMessage struct {
	BaseModel

	Kind                  string         `bun:"kind,notnull,default:'chat'"`
	ActorType             string         `bun:"actor_type,notnull,default:'system'"`
	ActorID               *uuid.UUID     `bun:"actor_id"`
	ActorDisplayName      string         `bun:"actor_display_name,default:''"`
	Body                  string         `bun:"body,notnull"`
	Internal              bool           `bun:"internal,notnull,default:false"`
	Source                string         `bun:"source,notnull,default:'alga'"`
	SlackChannelID        string         `bun:"slack_channel_id,default:''"`
	SlackMessageTs        string         `bun:"slack_message_ts,default:''"`
	SlackThreadTs         string         `bun:"slack_thread_ts,default:''"`
	ProviderMessageID     string         `bun:"provider_message_id,default:''"`
	LinkedInvestigationID string         `bun:"linked_investigation_id,default:''"`
	ParentMessageID       *uuid.UUID     `bun:"parent_message_id"`
	Metadata              map[string]any `bun:"metadata,type:jsonb,notnull,default:'{}'"`
	IncidentID            uuid.UUID      `bun:"incident_id,notnull"`
}
