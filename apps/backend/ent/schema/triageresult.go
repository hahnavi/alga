package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
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
		field.String("severity_input").Optional().Default(""),
		field.String("decision").NotEmpty(),
		field.Float("confidence").Default(0),
		field.String("severity_classified").Optional().Default(""),
		field.String("category").Optional().Default(""),
		field.Text("reasoning").Optional().Default(""),
		field.JSON("suggested_actions", []string{}).Optional(),
		field.JSON("enrichment", map[string]any{}).Optional(),
		field.JSON("context_used", map[string]any{}).Optional(),
		field.String("outcome").Default("pending"),
		field.String("overridden_to").Optional().Default(""),
		field.UUID("overridden_by", uuid.UUID{}).Optional(),
		field.Time("overridden_at").Optional().Nillable(),
		field.String("model_used").Optional().Default(""),
		field.Int64("triage_duration_ms").Default(0),
		field.String("trace_id").Optional().Default(""),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (TriageResult) Edges() []ent.Edge { return nil }

func (TriageResult) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("correlation_key"),
		index.Fields("decision"),
		index.Fields("outcome"),
		index.Fields("created_at"),
	}
}
