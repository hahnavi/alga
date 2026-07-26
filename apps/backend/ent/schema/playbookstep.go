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

type PlaybookStep struct {
	ent.Schema
}

func (PlaybookStep) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "playbook_steps"},
	}
}

func (PlaybookStep) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("playbook_id", uuid.UUID{}),
		field.Int("step_number").Positive(),
		field.String("title"),
		field.Text("description").Optional(),
		field.String("expected_duration").Optional(),
		field.String("command").Optional(),
		field.Time("created_at").Default(timeNow).Immutable(),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (PlaybookStep) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("playbook", Playbook.Type).Ref("steps").Field("playbook_id").Unique().Required(),
	}
}

func (PlaybookStep) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("playbook_id", "step_number").Unique(),
	}
}
