package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Notification struct {
	ent.Schema
}

func (Notification) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "notifications"},
	}
}

func (Notification) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("user_id").NotEmpty(),
		field.String("type").NotEmpty(),
		field.String("title").NotEmpty(),
		field.String("message").NotEmpty(),
		field.Bool("read").Default(false),
		field.String("resource_type").Optional().Default(""),
		field.String("resource_id").Optional().Default(""),
		field.String("triggered_by_user_id").Optional().Default(""),
		field.String("triggered_by_display_name").Optional().Default(""),
		field.Time("created_at").Default(timeNow),
	}
}

func (Notification) Edges() []ent.Edge {
	return nil
}

func (Notification) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("user_id", "read"),
	}
}
