package models

import (
	"time"

	"github.com/google/uuid"
)

type IncidentTimelineEntry struct {
	ID           uuid.UUID      `bun:"id,pk"`
	EventType    string         `bun:"event_type,notnull,default:'note_added'"`
	ActorID      *uuid.UUID     `bun:"actor_id"`
	ActorType    string         `bun:"actor_type,notnull,default:'system'"`
	Message      string         `bun:"message,notnull,default:''"`
	Metadata     map[string]any `bun:"metadata,type:jsonb,notnull,default:'{}'"`
	ICSEventType *string        `bun:"ics_event_type"`
	CreatedAt    time.Time      `bun:"created_at,notnull,default:current_timestamp"`
	IncidentID   uuid.UUID      `bun:"incident_id,notnull"`
}

func (*IncidentTimelineEntry) TableName() string { return "incident_timeline_entries" }
