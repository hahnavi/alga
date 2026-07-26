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

type ServiceDependency struct {
	ent.Schema
}

func (ServiceDependency) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "service_dependencies"},
	}
}

func (ServiceDependency) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("service_id", uuid.UUID{}),
		field.UUID("dependent_on_service_id", uuid.UUID{}),
		field.Enum("dependency_type").Values("depends_on", "hard", "soft").Default("depends_on"),
		field.Time("created_at").Default(timeNow),
	}
}

func (ServiceDependency) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("service", Service.Type).Ref("dependencies").Field("service_id").Unique().Required(),
		edge.From("dependent_on_service", Service.Type).Ref("depended_on_by").Field("dependent_on_service_id").Unique().Required(),
	}
}

func (ServiceDependency) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("service_id", "dependent_on_service_id").Unique(),
		index.Fields("dependent_on_service_id"),
	}
}
