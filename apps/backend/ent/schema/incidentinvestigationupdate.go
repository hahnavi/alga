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

type IncidentInvestigationUpdateEntry struct {
	ent.Schema
}

func (IncidentInvestigationUpdateEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "incident_investigation_updates"},
	}
}

func (IncidentInvestigationUpdateEntry) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("incident_investigation_uuid", uuid.UUID{}),
		field.String("type").NotEmpty(),
		field.String("message").NotEmpty(),
		field.String("source").NotEmpty(),
		field.Bool("internal").Default(false),
		field.Bool("edited").Default(false),
		field.String("user_id").Optional().Nillable(),
		field.String("username").Optional().Nillable(),
		field.String("mm_post_id").Optional().Default(""),
		field.String("slack_message_ts").Optional().Default(""),
		field.String("quoted_update_id").Optional().Nillable(),
		field.JSON("mentions", []string{}).Optional(),
		field.Time("created_at").Default(timeNow),
	}
}

func (IncidentInvestigationUpdateEntry) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("incident_investigation", IncidentInvestigation.Type).
			Ref("updates").
			Unique().
			Field("incident_investigation_uuid").
			Required(),
	}
}

func (IncidentInvestigationUpdateEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("incident_investigation_uuid"),
	}
}
