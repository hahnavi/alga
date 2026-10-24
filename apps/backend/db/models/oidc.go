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
	Scopes                []string `bun:"scopes,type:jsonb,notnull"`
	Enabled               bool     `bun:"enabled,notnull,default:true"`
}

type OIDCIdentity struct {
	BaseModel

	UserID     uuid.UUID `bun:"user_id,notnull"`
	ProviderID uuid.UUID `bun:"provider_id,notnull"`
	Subject    string    `bun:"subject,notnull"`
	Issuer     string    `bun:"issuer,notnull,default:''"`
	Email      string    `bun:"email,notnull,default:''"`
}

type SAMLProvider struct {
	BaseModel

	Name           string `bun:"name,notnull,unique"`
	IDPEntityID    string `bun:"idp_entity_id,notnull,default:''"`
	MetadataXML    string `bun:"metadata_xml,notnull,default:''"`
	EmailAttribute string `bun:"email_attribute,notnull,default:'email'"`
	Enabled        bool   `bun:"enabled,notnull,default:false"`
}

type SAMLIdentity struct {
	BaseModel

	UserID     uuid.UUID `bun:"user_id,notnull"`
	ProviderID uuid.UUID `bun:"provider_id,notnull"`
	Subject    string    `bun:"subject,notnull"`
	Email      string    `bun:"email,notnull,default:''"`
}
