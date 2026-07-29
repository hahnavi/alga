package models

import (
	"github.com/google/uuid"
)

type AgentDMMessage struct {
	BaseModel

	ChatID       string    `bun:"chat_id,notnull,default:'alga_dm'"`
	Role         string    `bun:"role,notnull"`
	Body         string    `bun:"body,notnull"`
	UserID       *string   `bun:"user_id"`
	Username     *string   `bun:"username"`
	Edited       bool      `bun:"edited,notnull,default:false"`
	AgentTokenID uuid.UUID `bun:"agent_token_id,notnull"`
}
