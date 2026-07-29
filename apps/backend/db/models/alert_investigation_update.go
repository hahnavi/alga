package models

import (
	"github.com/google/uuid"
)

type AlertInvestigationUpdate struct {
	BaseModel

	AlertInvestigationID uuid.UUID `bun:"alert_investigation_id,notnull"`
	Type                 string    `bun:"type,notnull"`
	Message              string    `bun:"message,notnull"`
	Source               string    `bun:"source,notnull"`
	Internal             bool      `bun:"internal,notnull,default:false"`
	Edited               bool      `bun:"edited,notnull,default:false"`
	UserID               *string   `bun:"user_id"`
	Username             *string   `bun:"username"`
	MMPostID             string    `bun:"mm_post_id"`
	SlackMessageTS       string    `bun:"slack_message_ts"`
	QuotedUpdateID       *string   `bun:"quoted_update_id"`
	Mentions             []string  `bun:"mentions,type:jsonb"`
}

func (*AlertInvestigationUpdate) TableName() string { return "alert_investigation_updates" }
