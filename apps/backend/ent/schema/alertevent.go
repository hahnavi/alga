package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type AlertEvent struct {
	ent.Schema
}

func (AlertEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "alert_events"},
	}
}

func (AlertEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("type").NotEmpty(),
		field.Time("timestamp").Default(timeNow),
		field.String("actor_username").Optional().Default(""),
		field.String("actor_display_name").Optional().Default(""),
		field.String("actor_user_id").Optional().Default(""),
		field.String("source").Optional().Default(""),
	}
}

func (AlertEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("alert", Alert.Type).
			Ref("events").
			Unique().
			Required(),
	}
}

func (AlertEvent) Indexes() []ent.Index {
	return []ent.Index{}
}
