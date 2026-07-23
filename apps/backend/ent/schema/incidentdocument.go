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

type IncidentDocument struct {
	ent.Schema
}

func (IncidentDocument) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "incident_documents"},
	}
}

func (IncidentDocument) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("section").Default("current_status"),
		field.Text("content").Default(""),
		field.Int("version").Default(1),
		field.Time("updated_at").Default(timeNow),
	}
}

func (IncidentDocument) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("incident", Incident.Type).Ref("documents").Unique().Required(),
		edge.From("updated_by", User.Type).Ref("document_edits").Unique(),
	}
}

func (IncidentDocument) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("section"),
	}
}
