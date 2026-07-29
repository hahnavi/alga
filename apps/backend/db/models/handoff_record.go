package models

import (
	"time"

	"github.com/google/uuid"
)

type HandoffRecord struct {
	BaseModel

	ScheduleID             uuid.UUID  `bun:"schedule_id,notnull"`
	OutgoingUserID         *uuid.UUID `bun:"outgoing_user_id"`
	IncomingUserID         *uuid.UUID `bun:"incoming_user_id"`
	HandoffAt              time.Time  `bun:"handoff_at,notnull"`
	Status                 string     `bun:"status,notnull,default:'pending'"`
	OutgoingNotes          string     `bun:"outgoing_notes"`
	IncomingNotes          string     `bun:"incoming_notes"`
	IncomingAcknowledgedAt *time.Time `bun:"incoming_acknowledged_at"`
	IncidentSummary        string     `bun:"incident_summary"`
}
