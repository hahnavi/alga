package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ScheduleOverride struct {
	bun.BaseModel `bun:"table:schedule_overrides"`

	ID         uuid.UUID  `bun:"id,pk"`
	ScheduleID uuid.UUID  `bun:"schedule_id,notnull"`
	UserID     uuid.UUID  `bun:"user_id,notnull"`
	StartAt    time.Time  `bun:"start_at,notnull"`
	EndAt      time.Time  `bun:"end_at,notnull"`
	CreatedBy  *uuid.UUID `bun:"created_by"`
	CreatedAt  time.Time  `bun:"created_at,notnull,default:current_timestamp"`
}
