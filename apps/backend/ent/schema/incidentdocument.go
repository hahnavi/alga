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
		field.UUID("incident_id", uuid.UUID{}),
		field.UUID("updated_by_id", uuid.UUID{}).Optional().Nillable(),
	}
}

func (IncidentDocument) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("incident", Incident.Type).Ref("documents").Unique().Required().Field("incident_id"),
		edge.From("updated_by", User.Type).Ref("document_edits").Unique().Field("updated_by_id"),
	}
}

func (IncidentDocument) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("incident_id", "section").Unique(),
		index.Fields("updated_by_id"),
	}
}
