package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type RouteRules struct {
	ent.Schema
}

func (RouteRules) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "route_rules"},
	}
}

func (RouteRules) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).StorageKey("id").Default(func() uuid.UUID {
			return uuid.UUID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		}),
		field.JSON("routes", []RouteConfig{}).Optional(),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (RouteRules) Edges() []ent.Edge {
	return nil
}
