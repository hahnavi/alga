package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type BaseModel struct {
	bun.BaseModel
	ID        uuid.UUID `bun:"id,pk"`
	CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp"`
}

// IDModel is a leaner base for append-only tables that carry their own
// authoritative timestamp column and therefore need neither created_at nor
// updated_at (and no updated_at trigger).
type IDModel struct {
	bun.BaseModel
	ID uuid.UUID `bun:"id,pk"`
}

type SoftDeleteModel struct {
	DeletedAt *time.Time `bun:"deleted_at,soft_delete"`
}

func NewUUID() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}
