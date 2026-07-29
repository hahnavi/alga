package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type StatusPage struct {
	bun.BaseModel `bun:"table:status_pages"`

	ID          uuid.UUID  `bun:"id,pk"`
	Name        string     `bun:"name,notnull"`
	Slug        string     `bun:"slug,notnull,unique"`
	Description string     `bun:"description,notnull,default:''"`
	Visibility  string     `bun:"visibility,notnull,default:'internal'"`
	Enabled     bool       `bun:"enabled,notnull,default:true"`
	OwnerTeamID *uuid.UUID `bun:"owner_team_id"`
	CreatedAt   time.Time  `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt   time.Time  `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*StatusPage) TableName() string { return "status_pages" }
