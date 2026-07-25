package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type Counter struct {
	ent.Schema
}

func (Counter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "counters"},
	}
}

func (Counter) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("id").NotEmpty(),
		field.Int64("seq").Default(0).NonNegative(),
	}
}

func (Counter) Edges() []ent.Edge {
	return nil
}
