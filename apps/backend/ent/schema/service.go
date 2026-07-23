package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Service struct {
	ent.Schema
}

func (Service) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "services"},
	}
}

func (Service) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").Unique().NotEmpty(),
		field.String("display_name").Default(""),
		field.String("description").Default(""),
		field.UUID("owner_team_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("escalation_policy_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("label_matchers", []map[string]any{}).Default([]map[string]any{}),
		field.Int("sla_response_minutes").Default(0),
		field.Int("sla_resolve_minutes").Default(0),
		field.String("status").Default("operational"),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (Service) Edges() []ent.Edge {
	return nil
}

func (Service) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
