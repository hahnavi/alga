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

type InvestigationThread struct {
	ent.Schema
}

func (InvestigationThread) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "investigation_threads"},
	}
}

func (InvestigationThread) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("thread_id").Unique().NotEmpty(),
		field.String("owner_type").NotEmpty(),
		field.String("owner_id").NotEmpty(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (InvestigationThread) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("messages", InvestigationThreadMessage.Type),
	}
}

func (InvestigationThread) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("owner_type", "owner_id").Unique(),
	}
}
