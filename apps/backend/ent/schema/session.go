package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Session struct {
	ent.Schema
}

func (Session) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sessions"},
	}
}

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("user_id", uuid.UUID{}),
		field.String("id_hash").Unique().NotEmpty(),
		field.String("refresh_token_hash").Optional(),
		field.JSON("prev_refresh_token_hashes", []string{}).Optional(),
		field.String("family_id").NotEmpty(),
		field.Time("created_at").Default(timeNow),
		field.Time("expires_at"),
		field.Time("last_used_at").Default(timeNow),
		field.String("ip").Optional().Default(""),
		field.String("user_agent").Optional().Default(""),
	}
}

func (Session) Edges() []ent.Edge {
	return nil
}

func (Session) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("refresh_token_hash"),
		index.Fields("user_id"),
		index.Fields("family_id"),
		index.Fields("expires_at"),
	}
}
