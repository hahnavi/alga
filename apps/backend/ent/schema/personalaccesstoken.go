package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PersonalAccessToken struct {
	ent.Schema
}

func (PersonalAccessToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "personal_access_tokens"},
	}
}

func (PersonalAccessToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("user_id", uuid.UUID{}),
		field.String("name").NotEmpty().MaxLen(128),
		field.String("token_hash").Unique().NotEmpty(),
		field.String("lookup_prefix").NotEmpty(),
		field.JSON("permissions", []string{}),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Bool("revoked").Default(false),
	}
}

func (PersonalAccessToken) Edges() []ent.Edge {
	return nil
}

func (PersonalAccessToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("lookup_prefix", "revoked"),
		index.Fields("user_id"),
	}
}
