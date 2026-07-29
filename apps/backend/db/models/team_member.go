package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type TeamMember struct {
	bun.BaseModel `bun:"table:team_members"`

	ID        uuid.UUID `bun:"id,pk"`
	TeamID    uuid.UUID `bun:"team_id,notnull"`
	UserID    uuid.UUID `bun:"user_id,notnull"`
	Role      string    `bun:"role,notnull,default:'member'"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp"`
}

func (*TeamMember) TableName() string { return "team_members" }
