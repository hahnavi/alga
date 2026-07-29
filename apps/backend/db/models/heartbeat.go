package models

import (
	"time"

	"github.com/google/uuid"
)

type Heartbeat struct {
	BaseModel

	Name            string            `bun:"name,notnull"`
	Description     string            `bun:"description"`
	IntervalSeconds int               `bun:"interval_seconds,notnull"`
	GraceSeconds    int               `bun:"grace_seconds,notnull,default:60"`
	Enabled         bool              `bun:"enabled,notnull,default:true"`
	OwnerTeamID     *uuid.UUID        `bun:"owner_team_id"`
	Status          string            `bun:"status,notnull,default:'healthy'"`
	Severity        string            `bun:"severity,notnull,default:'warning'"`
	Labels          map[string]string `bun:"labels,type:jsonb"`
	PingTokenHash   string            `bun:"ping_token_hash,notnull,unique"`
	LookupPrefix    string            `bun:"lookup_prefix,notnull"`
	LastPingAt      *time.Time        `bun:"last_ping_at"`
	ExpiresAt       *time.Time        `bun:"expires_at"`
	LastBreachAt    *time.Time        `bun:"last_breach_at"`
	CreatedBy       string            `bun:"created_by"`
}
