package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type WebhookToken struct {
	ent.Schema
}

func (WebhookToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "webhook_tokens"},
	}
}

func (WebhookToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").NotEmpty(),
		field.String("token_hash").Unique().NotEmpty().Sensitive(),
		field.String("lookup_prefix").NotEmpty(),
		field.Time("created_at").Default(timeNow),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable(),
		field.Bool("revoked").Default(false),
	}
}

func (WebhookToken) Edges() []ent.Edge {
	return nil
}

func (WebhookToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("lookup_prefix").
			Annotations(entsql.IndexWhere("revoked = false")),
		index.Fields("expires_at"),
	}
}
