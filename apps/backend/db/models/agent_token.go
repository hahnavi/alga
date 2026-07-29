package models

import "time"

type AgentToken struct {
	BaseModel

	Name                    string           `bun:"name,notnull"`
	AgentType               string           `bun:"agent_type,notnull,default:'hermes'"`
	TokenHash               string           `bun:"token_hash,notnull,unique"`
	LookupPrefix            string           `bun:"lookup_prefix,notnull"`
	LastUsedAt              *time.Time       `bun:"last_used_at"`
	ExpiresAt               *time.Time       `bun:"expires_at"`
	Revoked                 bool             `bun:"revoked,notnull,default:false"`
	Enabled                 bool             `bun:"enabled,notnull,default:true"`
	Scope                   string           `bun:"scope"`
	LabelSelectors          []RouteCondition `bun:"label_selectors,type:jsonb"`
	DefaultForInvestigation bool             `bun:"default_for_investigation,default:false"`
	Capabilities            []string         `bun:"capabilities,type:jsonb"`
}

func (*AgentToken) TableName() string { return "agent_tokens" }
