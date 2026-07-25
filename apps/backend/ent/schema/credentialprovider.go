package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CredentialProvider is a configurable secret backend. The built-in "internal"
// type stores secrets encrypted in the Alga database; external types
// (hashicorp_vault, aws_secrets_manager, gcp_secret_manager, azure_key_vault)
// proxy reads to a remote secret store. Connection details live in
// config_encrypted as an encrypted JSON blob.
type CredentialProvider struct {
	ent.Schema
}

func (CredentialProvider) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "credential_providers"},
	}
}

func (CredentialProvider) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").NotEmpty(),
		field.String("type").Default("internal"),
		// config_encrypted holds the provider-specific connection config as an
		// encrypted JSON blob (e.g. vault address + token, aws region). The
		// "internal" provider leaves this empty.
		field.String("config_encrypted").Default("").Sensitive(),
		field.Bool("enabled").Default(true),
		// system marks built-in providers that are seeded and cannot be removed
		// (e.g. the default "Alga Internal" provider). It is set only by the
		// seeder, never by the admin API.
		field.Bool("system").Default(false),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (CredentialProvider) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("shared_secrets", SharedSecret.Type).Annotations(entsql.Annotation{OnDelete: entsql.Restrict}),
	}
}

func (CredentialProvider) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("enabled"),
	}
}
