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

type Incident struct {
	ent.Schema
}

func (Incident) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "incidents"},
	}
}

func (Incident) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.Int64("incident_number").Unique().NonNegative(),
		field.String("title").Default(""),
		field.String("description").Default(""),
		field.String("summary").Default("").Optional(),
		field.Enum("status").Values("detected", "triaging", "active", "mitigated", "resolved", "closed", "cancelled").Default("detected"),
		field.Enum("severity").Values("critical", "high", "warning", "info").Default("warning"),
		field.Enum("impact_level").Values("high", "medium", "low").Default("medium"),
		field.Enum("priority").Values("P1", "P2", "P3", "P4", "P5").Default("P4"),
		field.Enum("incident_type").Values("real", "alert", "degradation").Default("real"),
		field.UUID("commander_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("communicator_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("on_call_responder_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("commander_assignee_type").Values("user", "agent").Default("user").Optional(),
		field.Enum("communicator_assignee_type").Values("user", "agent").Default("user").Optional(),
		field.UUID("service_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("escalation_policy_id", uuid.UUID{}).Optional().Nillable(),
		field.String("conference_url").Default(""),
		field.String("slack_channel_id").Optional().Nillable(),
		field.String("slack_channel_name").Optional().Default(""),
		field.Bool("slack_channel_archived").Optional().Default(false),
		field.String("war_room_channel_id").Optional().Nillable(),
		field.String("war_room_channel_provider").Optional().Nillable(),
		field.String("google_meet_space_name").Optional().Nillable(),
		field.String("status_page_incident_id").Default(""),
		field.Time("sla_target_respond_at").Optional().Nillable(),
		field.Time("sla_target_resolve_at").Optional().Nillable(),
		field.Time("sla_acknowledged_at").Optional().Nillable(),
		field.Time("sla_resolved_at").Optional().Nillable(),
		field.Time("started_at").Optional().Nillable(),
		field.Time("mitigated_at").Optional().Nillable(),
		field.Time("resolved_at").Optional().Nillable(),
		field.Time("closed_at").Optional().Nillable(),
		field.Time("triaged_at").Optional().Nillable(),
		field.JSON("triage_report", map[string]any{}).Optional(),
		field.Bool("auto_confirmed").Default(false),
		field.JSON("tags", []string{}).Default([]string{}),
		field.JSON("custom_fields", map[string]any{}).Default(map[string]any{}),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
		field.Time("deleted_at").Optional().Nillable(),
	}
}

func (Incident) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("alerts", Alert.Type),
		edge.To("incident_investigations", IncidentInvestigation.Type),
		edge.To("promoted_alert_investigations", AlertInvestigation.Type),
		edge.To("timeline", IncidentTimelineEntry.Type),
		edge.To("post_mortem", PostMortem.Type).Unique(),
		edge.To("ics_roles", ICSRoleAssignment.Type),
		edge.To("documents", IncidentDocument.Type),
		edge.To("coordination_messages", IncidentCoordinationMessage.Type),
		edge.To("coordination_tasks", CoordinationTask.Type),
		edge.From("commander", User.Type).Ref("commander_incidents").Field("commander_id").Unique(),
		edge.From("communicator", User.Type).Ref("communicator_incidents").Field("communicator_id").Unique(),
		edge.From("on_call_responder", User.Type).Ref("responder_incidents").Field("on_call_responder_id").Unique(),
		edge.From("service", Service.Type).Ref("incidents").Field("service_id").Unique(),
		edge.From("escalation_policy", EscalationPolicy.Type).Ref("incidents").Field("escalation_policy_id").Unique(),
	}
}

func (Incident) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("severity").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("priority").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("commander_id").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("communicator_id").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("on_call_responder_id").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("service_id").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("escalation_policy_id").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
	}
}
