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

type IncidentInvestigation struct {
	ent.Schema
}

func (IncidentInvestigation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "incident_investigations"},
	}
}

func (IncidentInvestigation) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("incident_investigation_id").Unique().NotEmpty(),
		field.UUID("incident_id", uuid.UUID{}).Optional().Nillable(),
		field.String("status").Default("pending"),
		field.String("agent_id").Optional().Default(""),
		field.String("agent_name").Optional().Default(""),
		field.String("agent_type").Optional().Default(""),
		field.String("primary_thread_id").Optional().Default(""),
		field.String("slack_channel_id").Optional().Default(""),
		field.String("slack_thread_ts").Optional().Default(""),
		field.String("mm_post_id").Optional().Default(""),
		field.String("mm_thread_id").Optional().Default(""),
		field.UUID("source_alert_investigation_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("summary", &IncidentInvestigationSummary{}).Optional(),
		field.JSON("findings", []InvestigationFinding{}).Optional(),
		field.JSON("evidence", []EvidenceItem{}).Optional(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.Time("started_at").Optional().Nillable(),
		field.Int64("investigating_duration_ms").Optional().Default(0),
		field.UUID("parent_investigation_id", uuid.UUID{}).Optional().Nillable(),
		field.String("assignee_type").Default("agent"),
		field.UUID("assignee_id", uuid.UUID{}).Optional().Nillable(),
	}
}

func (IncidentInvestigation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("updates", IncidentInvestigationUpdateEntry.Type),
		edge.To("promoted_alert_investigations", AlertInvestigation.Type),
		edge.To("child_investigations", IncidentInvestigation.Type),
		edge.From("incident", Incident.Type).
			Ref("incident_investigations").
			Unique().
			Field("incident_id"),
		edge.From("source_alert_investigation", AlertInvestigation.Type).
			Ref("incident_investigations").
			Unique().
			Field("source_alert_investigation_id"),
		edge.From("parent_investigation", IncidentInvestigation.Type).
			Ref("child_investigations").
			Unique().
			Field("parent_investigation_id"),
		edge.To("linked_coordination_tasks", CoordinationTask.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
	}
}

func (IncidentInvestigation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("source_alert_investigation_id"),
		index.Fields("incident_id", "status"),
		index.Fields("parent_investigation_id"),
		index.Fields("assignee_type", "assignee_id", "status"),
	}
}
