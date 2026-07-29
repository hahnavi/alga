package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// RouteRules is a singleton configuration row holding all notification routing rules.
type RouteRules struct {
	bun.BaseModel `bun:"table:route_rules"`

	ID        uuid.UUID     `bun:"id,pk"`
	Routes    []RouteConfig `bun:"routes,type:jsonb"`
	CreatedAt time.Time     `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt time.Time     `bun:"updated_at,notnull,default:current_timestamp"`
}
