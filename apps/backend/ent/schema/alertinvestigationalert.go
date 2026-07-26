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

type AlertInvestigationAlert struct {
	ent.Schema
}

func (AlertInvestigationAlert) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "alert_investigation_alerts"},
	}
}

func (AlertInvestigationAlert) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("alert_investigation_id", uuid.UUID{}),
		field.UUID("alert_id", uuid.UUID{}).Optional().Nillable(),
		field.String("fingerprint").NotEmpty(),
		field.Int64("alert_number").Optional().NonNegative(),
		field.String("status").Optional().Default(""),
		field.String("alertname").Optional().Default(""),
		field.String("namespace").Optional().Default(""),
		field.JSON("labels", map[string]string{}).Optional(),
		field.JSON("annotations", map[string]string{}).Optional(),
		field.Time("starts_at").Optional().Nillable(),
		field.Time("ends_at").Optional().Nillable(),
		field.String("generator_url").Optional().Default(""),
		field.String("summary").Optional().Default(""),
		field.Bool("current").Default(true),
	}
}

func (AlertInvestigationAlert) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("alert_investigation", AlertInvestigation.Type).
			Ref("alerts").
			Unique().
			Field("alert_investigation_id").
			Required(),
		edge.From("alert", Alert.Type).
			Ref("alert_investigation_alerts").
			Unique().
			Field("alert_id"),
	}
}

func (AlertInvestigationAlert) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("fingerprint"),
		index.Fields("alert_investigation_id"),
		index.Fields("alert_number", "current").
			Annotations(entsql.IndexWhere("current = true AND alert_number > 0")).
			Unique(),
		index.Fields("alert_id", "current").
			Annotations(entsql.IndexWhere("current = true AND alert_id IS NOT NULL")).
			Unique(),
	}
}
