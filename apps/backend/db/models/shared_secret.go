package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type SharedSecret struct {
	bun.BaseModel `bun:"table:shared_secrets"`

	ID              uuid.UUID   `bun:"id,pk"`
	ProviderID      uuid.UUID   `bun:"provider_id,notnull"`
	Name            string      `bun:"name,notnull"`
	SecretID        string      `bun:"secret_id,notnull"`
	Description     string      `bun:"description,notnull,default:''"`
	RemoteRef       string      `bun:"remote_ref,notnull,default:''"`
	ValueEncrypted  string      `bun:"value_encrypted,notnull,default:''"`
	ValueConfigured bool        `bun:"value_configured,notnull,default:false"`
	AllowedAgentIDs []uuid.UUID `bun:"allowed_agent_ids,type:jsonb"`
	CreatedAt       time.Time   `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt       time.Time   `bun:"updated_at,notnull,default:current_timestamp"`
}
