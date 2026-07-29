package models

import (
	"github.com/google/uuid"
)

type OIDCProvider struct {
	BaseModel

	Name                  string   `bun:"name,notnull,unique"`
	Issuer                string   `bun:"issuer,notnull,unique"`
	ClientID              string   `bun:"client_id,notnull"`
	ClientSecretEncrypted string   `bun:"client_secret_encrypted,notnull,default:''"`
	Scopes                []string `bun:"scopes,type:jsonb,notnull,default:'[\"openid\",\"email\",\"profile\"]'"`
	Enabled               bool     `bun:"enabled,notnull,default:true"`
}

func (*OIDCProvider) TableName() string { return "oidc_providers" }

type OIDCIdentity struct {
	BaseModel

	UserID     uuid.UUID `bun:"user_id,notnull"`
	ProviderID uuid.UUID `bun:"provider_id,notnull"`
	Subject    string    `bun:"subject,notnull"`
	Issuer     string    `bun:"issuer,notnull,default:''"`
	Email      string    `bun:"email,notnull,default:''"`
}

func (*OIDCIdentity) TableName() string { return "oidc_identities" }
