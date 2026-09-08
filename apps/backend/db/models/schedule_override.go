package models

import (
	"github.com/google/uuid"
	"time"
)

type ScheduleOverride struct {
	CreatedOnlyModel `bun:"table:schedule_overrides"`

	ScheduleID uuid.UUID  `bun:"schedule_id,notnull"`
	UserID     uuid.UUID  `bun:"user_id,notnull"`
	StartAt    time.Time  `bun:"start_at,notnull"`
	EndAt      time.Time  `bun:"end_at,notnull"`
	CreatedBy  *uuid.UUID `bun:"created_by"`
}
