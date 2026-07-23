package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ActionItem struct {
	ent.Schema
}

func (ActionItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "action_items"},
	}
}

func (ActionItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("post_mortem_id", uuid.UUID{}),
		field.Text("description").NotEmpty(),
		field.String("type").Default("investigate"),
		field.String("assignee_name").Default("").Optional(),
		field.UUID("assignee_id", uuid.UUID{}).Optional().Nillable(),
		field.String("status").Default("open"),
		field.String("priority").Default("medium"),
		field.Time("due_date").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (ActionItem) Edges() []ent.Edge {
	return nil
}

func (ActionItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("post_mortem_id"),
		index.Fields("assignee_id", "status"),
		index.Fields("status"),
		index.Fields("type"),
	}
}
