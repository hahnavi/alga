package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PostMortem struct {
	ent.Schema
}

func (PostMortem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "post_mortems"},
	}
}

func (PostMortem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("incident_id", uuid.UUID{}).Unique(),
		field.String("title").Default(""),
		field.String("status").Default("draft"),
		field.Text("summary").Default(""),
		field.JSON("timeline", []map[string]any{}).Optional(),
		field.Text("root_cause").Default(""),
		field.JSON("contributing_factors", []string{}).Optional(),
		field.Text("impact").Default(""),
		field.Text("lessons_learned").Default(""),
		field.Text("what_went_well").Default(""),
		field.Text("what_went_wrong").Default(""),
		field.Bool("blameless_confirmed").Default(false),
		field.Text("blameless_notes").Default(""),
		field.UUID("approved_by_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("published_at").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (PostMortem) Edges() []ent.Edge {
	return nil
}

func (PostMortem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
