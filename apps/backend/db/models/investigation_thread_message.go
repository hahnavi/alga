package models

import (
	"github.com/google/uuid"
)

type InvestigationThreadMessage struct {
	BaseModel

	ThreadID         uuid.UUID `bun:"thread_id,notnull"`
	Type             string    `bun:"type,notnull,default:'comment'"`
	Source           string    `bun:"source,notnull,default:'user'"`
	Message          string    `bun:"message,notnull"`
	Internal         bool      `bun:"internal,notnull,default:false"`
	Edited           bool      `bun:"edited,notnull,default:false"`
	UserID           string    `bun:"user_id,default:''"`
	Username         string    `bun:"username,default:''"`
	AgentType        string    `bun:"agent_type,default:''"`
	MMPostID         string    `bun:"mm_post_id,default:''"`
	SlackMessageTs   string    `bun:"slack_message_ts,default:''"`
	ReplyToMessageID string    `bun:"reply_to_message_id,default:''"`
	Mentions         []string  `bun:"mentions,type:jsonb"`
}

func (*InvestigationThreadMessage) TableName() string { return "investigation_thread_messages" }
