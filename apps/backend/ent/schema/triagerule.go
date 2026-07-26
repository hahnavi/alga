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

type TriageRule struct {
	ent.Schema
}

func (TriageRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "triage_rules"},
	}
}

func (TriageRule) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").NotEmpty(),
		field.String("description").Optional().Default(""),
		field.JSON("conditions", []map[string]any{}).Optional(),
		field.Enum("match_mode").Values("all", "any").Default("all"),
		field.Enum("decision").Values("investigate", "auto_resolve", "suppress", "escalate", "enrich_only"),
		field.Enum("severity").Values("critical", "high", "warning", "info", "low").Optional().Nillable(),
		field.Enum("category").Values("infrastructure", "application", "network", "security", "other").Optional().Nillable(),
		field.JSON("enrichment", map[string]any{}).Optional(),
		field.Int("priority").Default(0).NonNegative(),
		field.Bool("enabled").Default(true),
		field.UUID("created_by", uuid.UUID{}).Optional(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (TriageRule) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("created_by_user", User.Type).Ref("triage_rules").Field("created_by").Unique(),
	}
}

func (TriageRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "priority"),
		index.Fields("created_by"),
	}
}
