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

type Alert struct {
	ent.Schema
}

func (Alert) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "alerts"},
	}
}

func (Alert) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("fingerprint").NotEmpty(),
		field.Enum("status").Values("firing", "resolved").Default("firing"),
		field.Bool("acknowledged").Default(false),
		field.Bool("silenced").Default(false),
		field.JSON("labels", map[string]string{}).Default(map[string]string{}),
		field.JSON("annotations", map[string]string{}).Default(map[string]string{}),
		field.JSON("values", map[string]any{}).Optional(),
		field.Time("starts_at").Default(timeNow),
		field.Time("ends_at").Optional().Nillable(),
		field.String("generator_url").Optional().Default(""),
		field.Int64("alert_number").Unique().Optional().NonNegative(),
		field.UUID("triage_result_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("enrichment", map[string]any{}).Optional(),
		field.String("triage_category").Optional().Default(""),
		field.String("severity_classified").Optional().Default(""),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
		field.Time("deleted_at").Optional().Nillable(),
	}
}

func (Alert) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("incidents", Incident.Type).Ref("alerts"),
		edge.To("alert_investigation_alerts", AlertInvestigationAlert.Type),
		edge.To("events", AlertEvent.Type),
		edge.To("delivery_targets", DeliveryTarget.Type),
		edge.From("triage_result", TriageResult.Type).Ref("alerts").Field("triage_result_id").Unique(),
	}
}

func (Alert) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("fingerprint", "updated_at"),
		index.Fields("fingerprint").
			Unique().
			Annotations(entsql.IndexWhere("status != 'resolved' AND deleted_at IS NULL")),
		index.Fields("updated_at").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("status", "created_at").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("triage_result_id").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
	}
}
