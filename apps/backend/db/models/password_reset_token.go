package models

import (
	"time"

	"github.com/google/uuid"
)

type PasswordResetToken struct {
	ID        uuid.UUID `bun:"id,pk"`
	UserID    uuid.UUID `bun:"user_id,notnull"`
	TokenHash string    `bun:"token_hash,notnull,unique"`
	ExpiresAt time.Time `bun:"expires_at,notnull"`
	Used      bool      `bun:"used,notnull,default:false"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp"`
}
