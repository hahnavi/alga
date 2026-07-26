package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type SystemConfig struct {
	ent.Schema
}

func (SystemConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "system_config"},
	}
}

func (SystemConfig) Fields() []ent.Field {
	return []ent.Field{
		// Singleton row; id is fixed so store lookups are deterministic.
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID {
			return uuid.UUID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		}).StorageKey("id"),
		field.JSON("config", map[string]any{}).Optional(),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (SystemConfig) Edges() []ent.Edge {
	return nil
}
