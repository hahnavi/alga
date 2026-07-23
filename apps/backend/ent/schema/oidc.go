package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type OIDCProvider struct {
	ent.Schema
}

func (OIDCProvider) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oidc_providers"},
	}
}

func (OIDCProvider) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").NotEmpty(),
		field.String("issuer").NotEmpty(),
		field.String("client_id").NotEmpty(),
		field.String("client_secret_encrypted").Default(""),
		field.JSON("scopes", []string{}).Default([]string{"openid", "email", "profile"}),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (OIDCProvider) Edges() []ent.Edge {
	return nil
}

func (OIDCProvider) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
	}
}

type OIDCIdentity struct {
	ent.Schema
}

func (OIDCIdentity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "oidc_identities"},
	}
}

func (OIDCIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("user_id", uuid.UUID{}),
		field.UUID("provider_id", uuid.UUID{}),
		field.String("subject").NotEmpty(),
		field.String("issuer").Default(""),
		field.String("email").Default(""),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (OIDCIdentity) Edges() []ent.Edge {
	return nil
}

func (OIDCIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "subject").Unique(),
		index.Fields("user_id"),
	}
}
