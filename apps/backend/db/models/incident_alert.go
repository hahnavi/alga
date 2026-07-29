package models

import "github.com/google/uuid"

// IncidentAlert is the incident↔alert M2M join table. It has a composite
// primary key and no surrogate id or timestamps.
type IncidentAlert struct {
	IncidentID uuid.UUID `bun:"incident_id,pk"`
	AlertID    uuid.UUID `bun:"alert_id,pk"`
}
