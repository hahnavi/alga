package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type DeliveryTarget struct {
	ent.Schema
}

func (DeliveryTarget) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "delivery_targets"},
	}
}

func (DeliveryTarget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("provider").NotEmpty(),
		field.String("channel").NotEmpty(),
		field.String("channel_name").Optional().Default(""),
		field.String("post_id").Optional().Default(""),
	}
}

func (DeliveryTarget) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("alert", Alert.Type).
			Ref("delivery_targets").
			Unique().
			Required(),
	}
}

func (DeliveryTarget) Indexes() []ent.Index {
	return []ent.Index{}
}
