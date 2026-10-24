package models

import "github.com/google/uuid"

type LDAPProvider struct {
	BaseModel
	Name                  string `bun:"name,notnull,unique"`
	URL                   string `bun:"url,notnull"`
	BaseDN                string `bun:"base_dn,notnull"`
	BindDN                string `bun:"bind_dn,notnull"`
	BindPasswordEncrypted string `bun:"bind_password_encrypted,notnull"`
	LoginAttribute        string `bun:"login_attribute,notnull"`
	IDAttribute           string `bun:"id_attribute,notnull"`
	EmailAttribute        string `bun:"email_attribute,notnull"`
	StartTLS              bool   `bun:"start_tls,notnull,default:false"`
	Enabled               bool   `bun:"enabled,notnull,default:false"`
}

type LDAPIdentity struct {
	BaseModel
	UserID     uuid.UUID `bun:"user_id,notnull"`
	ProviderID uuid.UUID `bun:"provider_id,notnull"`
	Subject    string    `bun:"subject,notnull"`
}
