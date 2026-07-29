package models

import (
	"time"

	"github.com/google/uuid"
)

type AuditLog struct {
	IDModel

	Timestamp  time.Time      `bun:"timestamp,notnull,default:current_timestamp"`
	Event      string         `bun:"event,notnull"`
	UserID     *uuid.UUID     `bun:"user_id"`
	Username   string         `bun:"username"`
	IP         string         `bun:"ip"`
	UserAgent  string         `bun:"user_agent"`
	Success    bool           `bun:"success,notnull,default:true"`
	Details    map[string]any `bun:"details,type:jsonb"`
	RequestID  string         `bun:"request_id"`
	EntityType string         `bun:"entity_type"`
	EntityID   *uuid.UUID     `bun:"entity_id"`
}
