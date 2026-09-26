package models

import (
	"github.com/google/uuid"
)

type StatusPage struct {
	BaseModel `bun:"table:status_pages"`

	Name        string     `bun:"name,notnull"`
	Slug        string     `bun:"slug,notnull,unique"`
	Description string     `bun:"description,notnull,default:''"`
	Visibility  string     `bun:"visibility,notnull,default:'internal'"`
	Enabled     bool       `bun:"enabled,notnull,default:true"`
	OwnerTeamID *uuid.UUID `bun:"owner_team_id"`
}
