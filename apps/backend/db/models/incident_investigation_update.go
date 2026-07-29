package models

import (
	"time"

	"github.com/google/uuid"
)

type IncidentInvestigationUpdate struct {
	ID                      uuid.UUID `bun:"id,pk"`
	IncidentInvestigationID uuid.UUID `bun:"incident_investigation_id,notnull"`
	Type                    string    `bun:"type,notnull"`
	Message                 string    `bun:"message,notnull"`
	Source                  string    `bun:"source,notnull"`
	Internal                bool      `bun:"internal,notnull,default:false"`
	Edited                  bool      `bun:"edited,notnull,default:false"`
	UserID                  *string   `bun:"user_id"`
	Username                *string   `bun:"username"`
	MMPostID                string    `bun:"mm_post_id,default:''"`
	SlackMessageTs          string    `bun:"slack_message_ts,default:''"`
	QuotedUpdateID          *string   `bun:"quoted_update_id"`
	Mentions                []string  `bun:"mentions,type:jsonb"`
	CreatedAt               time.Time `bun:"created_at,notnull,default:current_timestamp"`
}
