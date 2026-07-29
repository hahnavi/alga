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

type SoftDeleteModel struct {
	DeletedAt *time.Time `bun:"deleted_at,soft_delete"`
}

func NewUUID() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}
