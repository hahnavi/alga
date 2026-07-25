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

type AlertInvestigation struct {
	ent.Schema
}

func (AlertInvestigation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "alert_investigations"},
	}
}

func (AlertInvestigation) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("alert_investigation_id").Unique().NotEmpty(),
		field.String("correlation_key").Optional().Default(""),
		field.String("status").Default("pending"),
		field.String("agent_id").Optional().Default(""),
		field.String("agent_name").Optional().Default(""),
		field.String("agent_type").Optional().Default(""),
		field.String("primary_thread_id").Optional().Default(""),
		field.String("slack_channel_id").Optional().Default(""),
		field.String("slack_thread_ts").Optional().Default(""),
		field.String("mm_post_id").Optional().Default(""),
		field.String("mm_thread_id").Optional().Default(""),
		field.UUID("promoted_incident_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("promoted_incident_investigation_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("summary", &AlertInvestigationSummary{}).Optional(),
		field.JSON("findings", []InvestigationFinding{}).Optional(),
		field.JSON("evidence", []EvidenceItem{}).Optional(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
		field.Time("completed_at").Optional().Nillable(),
		field.String("completed_reason").Optional().Default(""),
		field.String("completed_by_type").Optional().Default(""),
		field.String("completed_by_id").Optional().Default(""),
		field.String("completed_by_name").Optional().Default(""),
		field.Time("started_at").Optional().Nillable(),
		field.Int64("investigating_duration_ms").Optional().Default(0),
		field.String("primary_alert_fingerprint").Default(""),
		field.Int64("primary_alert_number").Optional().NonNegative(),
		field.String("escalation_level").Optional().Default(""),
		field.UUID("triage_result_id", uuid.UUID{}).Optional().Nillable(),
		field.String("triage_decision").Optional().Default(""),
		field.JSON("triage_enrichment", map[string]any{}).Optional(),
		field.String("assignee_type").Default("agent"),
		field.UUID("assignee_id", uuid.UUID{}).Optional().Nillable(),
	}
}

func (AlertInvestigation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("alerts", AlertInvestigationAlert.Type),
		edge.To("updates", AlertInvestigationUpdateEntry.Type),
		edge.To("events", AlertInvestigationEvent.Type),
		edge.To("incident_investigations", IncidentInvestigation.Type),
		edge.From("promoted_incident", Incident.Type).
			Ref("promoted_alert_investigations").
			Unique().
			Field("promoted_incident_id"),
		edge.From("promoted_incident_investigation", IncidentInvestigation.Type).
			Ref("promoted_alert_investigations").
			Unique().
			Field("promoted_incident_investigation_id"),
		edge.From("triage_result", TriageResult.Type).Ref("alert_investigations").Field("triage_result_id").Unique(),
	}
}

func (AlertInvestigation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("correlation_key", "status"),
		index.Fields("promoted_incident_id"),
		index.Fields("promoted_incident_investigation_id"),
		index.Fields("primary_alert_fingerprint"),
		index.Fields("primary_alert_number"),
		index.Fields("triage_result_id"),
		index.Fields("assignee_type", "assignee_id", "status"),
	}
}
