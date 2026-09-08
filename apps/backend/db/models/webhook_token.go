package models

import (
	"time"
)

type WebhookToken struct {
	CreatedOnlyModel `bun:"table:webhook_tokens"`

	Name         string     `bun:"name,notnull"`
	TokenHash    string     `bun:"token_hash,notnull,unique"`
	LookupPrefix string     `bun:"lookup_prefix,notnull"`
	LastUsedAt   *time.Time `bun:"last_used_at"`
	ExpiresAt    *time.Time `bun:"expires_at"`
}
