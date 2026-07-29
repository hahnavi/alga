package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Session struct {
	bun.BaseModel `bun:"table:sessions"`

	ID                     uuid.UUID `bun:"id,pk"`
	UserID                 uuid.UUID `bun:"user_id,notnull"`
	IDHash                 string    `bun:"id_hash,notnull,unique"`
	RefreshTokenHash       string    `bun:"refresh_token_hash"`
	PrevRefreshTokenHashes []string  `bun:"prev_refresh_token_hashes,type:jsonb"`
	FamilyID               string    `bun:"family_id,notnull"`
	CreatedAt              time.Time `bun:"created_at,notnull,default:current_timestamp"`
	ExpiresAt              time.Time `bun:"expires_at,notnull"`
	LastUsedAt             time.Time `bun:"last_used_at,notnull"`
	IP                     string    `bun:"ip,default:''"`
	UserAgent              string    `bun:"user_agent,default:''"`
}
