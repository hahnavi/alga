package models

import (
	"github.com/google/uuid"
)

type TeamMember struct {
	CreatedOnlyModel `bun:"table:team_members"`

	TeamID uuid.UUID `bun:"team_id,notnull"`
	UserID uuid.UUID `bun:"user_id,notnull"`
	Role   string    `bun:"role,notnull,default:'member'"`
}
