package models

import (
	"time"

	"github.com/google/uuid"
)

type PersonalAccessToken struct {
	ID           uuid.UUID  `bun:"id,pk"`
	UserID       uuid.UUID  `bun:"user_id,notnull"`
	Name         string     `bun:"name,notnull"`
	TokenHash    string     `bun:"token_hash,notnull,unique"`
	LookupPrefix string     `bun:"lookup_prefix,notnull"`
	Permissions  []string   `bun:"permissions,type:jsonb,notnull"`
	ExpiresAt    *time.Time `bun:"expires_at"`
	LastUsedAt   *time.Time `bun:"last_used_at"`
	CreatedAt    time.Time  `bun:"created_at,notnull,default:current_timestamp"`
	Revoked      bool       `bun:"revoked,notnull,default:false"`
}

func (*PersonalAccessToken) TableName() string { return "personal_access_tokens" }
