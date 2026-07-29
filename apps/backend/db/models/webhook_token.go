package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type WebhookToken struct {
	bun.BaseModel `bun:"table:webhook_tokens"`

	ID           uuid.UUID  `bun:"id,pk"`
	Name         string     `bun:"name,notnull"`
	TokenHash    string     `bun:"token_hash,notnull,unique"`
	LookupPrefix string     `bun:"lookup_prefix,notnull"`
	CreatedAt    time.Time  `bun:"created_at,notnull,default:current_timestamp"`
	LastUsedAt   *time.Time `bun:"last_used_at"`
	ExpiresAt    *time.Time `bun:"expires_at"`
	Revoked      bool       `bun:"revoked,notnull,default:false"`
}

func (*WebhookToken) TableName() string { return "webhook_tokens" }
