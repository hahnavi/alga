package models

import (
	"time"

	"github.com/google/uuid"
)

type AlertEvent struct {
	IDModel

	Type             string    `bun:"type,notnull"`
	Timestamp        time.Time `bun:"timestamp,notnull,default:current_timestamp"`
	ActorUsername    string    `bun:"actor_username"`
	ActorDisplayName string    `bun:"actor_display_name"`
	ActorUserID      string    `bun:"actor_user_id"`
	Source           string    `bun:"source"`
	AlertID          uuid.UUID `bun:"alert_id,notnull"`
}
