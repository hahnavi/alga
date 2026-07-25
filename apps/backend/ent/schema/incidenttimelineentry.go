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

type IncidentTimelineEntry struct {
	ent.Schema
}

func (IncidentTimelineEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "incident_timeline_entries"},
	}
}

func (IncidentTimelineEntry) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("event_type").Default("note_added"),
		field.UUID("actor_id", uuid.UUID{}).Optional().Nillable(),
		field.String("actor_type").Default("system"),
		field.String("message").Default(""),
		field.JSON("metadata", map[string]any{}).Default(map[string]any{}),
		field.String("ics_event_type").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.UUID("incident_id", uuid.UUID{}),
	}
}

func (IncidentTimelineEntry) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("incident", Incident.Type).Ref("timeline").Unique().Required().Field("incident_id"),
	}
}

func (IncidentTimelineEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("incident_id", "created_at"),
	}
}
