package models

import (
	"time"

	"github.com/google/uuid"
)

type NotificationDeliveryLog struct {
	ID               uuid.UUID  `bun:"id,pk"`
	UserID           uuid.UUID  `bun:"user_id,notnull"`
	IncidentID       *uuid.UUID `bun:"incident_id"`
	NotificationType string     `bun:"notification_type,notnull"`
	Channel          string     `bun:"channel,notnull"`
	Status           string     `bun:"status,notnull,default:'sent'"`
	ErrorMessage     string     `bun:"error_message,default:''"`
	CreatedAt        time.Time  `bun:"created_at,notnull,default:current_timestamp"`
}

func (*NotificationDeliveryLog) TableName() string { return "notification_delivery_logs" }
