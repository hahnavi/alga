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

type TriageResult struct {
	ent.Schema
}

func (TriageResult) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "triage_results"},
	}
}

func (TriageResult) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.Int64("triage_number").NonNegative().Unique(),
		field.String("correlation_key").NotEmpty(),
		field.Int("alert_count").Default(0),
		field.JSON("alert_fingerprints", []string{}).Optional(),
		field.JSON("alert_labels", map[string]string{}).Optional(),
		field.Enum("severity_input").Values("critical", "high", "warning", "info", "low").Optional().Nillable(),
		field.Enum("decision").Values("investigate", "auto_resolve", "suppress", "escalate", "enrich_only"),
		field.Float("confidence").Default(0),
		field.Enum("severity_classified").Values("critical", "high", "warning", "info", "low").Optional().Nillable(),
		field.Enum("category").Values("infrastructure", "application", "network", "security", "other").Optional().Nillable(),
		field.Text("reasoning").Optional().Default(""),
		field.JSON("suggested_actions", []string{}).Optional(),
		field.JSON("enrichment", map[string]any{}).Optional(),
		field.JSON("context_used", map[string]any{}).Optional(),
		field.Enum("outcome").Values("pending", "confirmed", "overridden").Default("pending"),
		field.Enum("overridden_to").Values("investigate", "auto_resolve", "suppress", "escalate", "enrich_only").Optional().Nillable(),
		field.UUID("overridden_by", uuid.UUID{}).Optional(),
		field.Time("overridden_at").Optional().Nillable(),
		field.String("model_used").Optional().Default(""),
		field.Int64("triage_duration_ms").Default(0).NonNegative(),
		field.String("trace_id").Optional().Default(""),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (TriageResult) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("alerts", Alert.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("alert_investigations", AlertInvestigation.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.From("overridden_by_user", User.Type).Ref("triage_overrides").Field("overridden_by").Unique(),
	}
}

func (TriageResult) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("correlation_key"),
		index.Fields("decision"),
		index.Fields("outcome"),
		index.Fields("created_at"),
		index.Fields("overridden_by"),
	}
}
