package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ScheduleLayer struct {
	bun.BaseModel `bun:"table:schedule_layers"`

	ID uuid.UUID `bun:"id,pk"`
	// ScheduleID references the owning on_call_schedules row.
	ScheduleID uuid.UUID `bun:"schedule_id,notnull"`
	Name       string    `bun:"name,notnull,default:''"`
	// RotationType is hourly|daily|weekly|monthly, enforced by the
	// schedule_layers_rotation_type_check constraint since migration 00016.
	RotationType     string     `bun:"rotation_type,notnull,default:'weekly'"`
	RotationInterval int        `bun:"rotation_interval,notnull,default:1"`
	StartDate        time.Time  `bun:"start_date,notnull"`
	EndDate          *time.Time `bun:"end_date"`
	Timezone         string     `bun:"timezone,notnull,default:'UTC'"`
	StartTime        string     `bun:"start_time,notnull,default:'00:00'"`
	EndTime          string     `bun:"end_time,notnull,default:''"`
	DaysOfWeek       []string   `bun:"days_of_week,type:jsonb,notnull,default:'[]'"`
	Priority         int        `bun:"priority,notnull,default:0"`
	UserIDs          []string   `bun:"user_ids,type:jsonb,notnull,default:'[]'"`
	CreatedAt        time.Time  `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt        time.Time  `bun:"updated_at,notnull,default:current_timestamp"`
}
