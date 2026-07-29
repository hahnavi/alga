package models

import (
	"time"

	"github.com/google/uuid"
)

type Outbox struct {
	ID            uuid.UUID  `bun:"id,pk"`
	EventType     string     `bun:"event_type,notnull"`
	AggregateID   string     `bun:"aggregate_id,notnull,default:''"`
	Exchange      string     `bun:"exchange,notnull"`
	RoutingKey    string     `bun:"routing_key,notnull,default:''"`
	Payload       []byte     `bun:"payload,notnull"`
	Status        string     `bun:"status,notnull,default:'pending'"`
	EventID       string     `bun:"event_id,notnull,default:''"`
	RetryCount    int        `bun:"retry_count,notnull,default:0"`
	CreatedAt     time.Time  `bun:"created_at,notnull,default:current_timestamp"`
	PublishedAt   *time.Time `bun:"published_at"`
	NextAttemptAt *time.Time `bun:"next_attempt_at"`
}

func (*Outbox) TableName() string { return "outboxes" }
