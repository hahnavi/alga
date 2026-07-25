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
		field.String("token_hash").Unique().NotEmpty().Sensitive(),
		field.String("lookup_prefix").NotEmpty(),
		field.JSON("permissions", []string{}),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Bool("revoked").Default(false),
	}
}

func (PersonalAccessToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("personal_access_tokens").Field("user_id").Unique().Required(),
	}
}

func (PersonalAccessToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("lookup_prefix").
			Annotations(entsql.IndexWhere("revoked = false")),
		index.Fields("user_id"),
		index.Fields("expires_at"),
	}
}
