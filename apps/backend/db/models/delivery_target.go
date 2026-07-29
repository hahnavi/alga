package models

import (
	"github.com/google/uuid"
)

type DeliveryTarget struct {
	BaseModel

	Provider    string    `bun:"provider,notnull"`
	Channel     string    `bun:"channel,notnull"`
	ChannelName string    `bun:"channel_name"`
	PostID      string    `bun:"post_id"`
	AlertID     uuid.UUID `bun:"alert_id,notnull"`
}

func (*DeliveryTarget) TableName() string { return "delivery_targets" }
