package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Team struct {
	bun.BaseModel `bun:"table:teams"`

	ID          uuid.UUID `bun:"id,pk"`
	Name        string    `bun:"name,notnull,unique"`
	Description string    `bun:"description,notnull,default:''"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt   time.Time `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*Team) TableName() string { return "teams" }
