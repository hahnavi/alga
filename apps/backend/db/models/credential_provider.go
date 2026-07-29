package models

type CredentialProvider struct {
	BaseModel

	Name            string `bun:"name,notnull,unique"`
	Type            string `bun:"type,notnull,default:'internal'"`
	ConfigEncrypted string `bun:"config_encrypted"`
	Enabled         bool   `bun:"enabled,notnull,default:true"`
	System          bool   `bun:"system,notnull,default:false"`
}

func (*CredentialProvider) TableName() string { return "credential_providers" }
