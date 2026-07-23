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

type AlertInvestigationEvent struct {
	ent.Schema
}

func (AlertInvestigationEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "alert_investigation_events"},
	}
}

func (AlertInvestigationEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("alert_investigation_uuid", uuid.UUID{}),
		field.String("event_type").NotEmpty(),
		field.String("reason").Optional().Default(""),
		field.String("actor_type").Optional().Default("system"),
		field.String("actor_id").Optional().Default(""),
		field.String("actor_name").Optional().Default(""),
		field.String("agent_id").Optional().Default(""),
		field.String("agent_name").Optional().Default(""),
		field.String("agent_type").Optional().Default(""),
		field.JSON("metadata", map[string]any{}).Optional(),
		field.Time("created_at").Default(timeNow),
	}
}

func (AlertInvestigationEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("alert_investigation", AlertInvestigation.Type).
			Ref("events").
			Unique().
			Field("alert_investigation_uuid").
			Required(),
	}
}

func (AlertInvestigationEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("alert_investigation_uuid", "created_at"),
		index.Fields("event_type"),
	}
}
