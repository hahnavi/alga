package models

import (
	"github.com/google/uuid"
)

type SharedSecret struct {
	BaseModel `bun:"table:shared_secrets"`

	ProviderID      uuid.UUID   `bun:"provider_id,notnull"`
	Name            string      `bun:"name,notnull"`
	SecretID        string      `bun:"secret_id,notnull"`
	Description     string      `bun:"description,notnull,default:''"`
	RemoteRef       string      `bun:"remote_ref,notnull,default:''"`
	ValueEncrypted  string      `bun:"value_encrypted,notnull,default:''"`
	ValueConfigured bool        `bun:"value_configured,notnull,default:false"`
	AllowedAgentIDs []uuid.UUID `bun:"allowed_agent_ids,type:jsonb"`
}
