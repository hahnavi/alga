package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type AgentAsk struct {
	ent.Schema
}

func (AgentAsk) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agent_asks"},
	}
}

func (AgentAsk) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("from_agent_id", uuid.UUID{}),
		field.String("from_agent_name").NotEmpty(),
		field.String("from_agent_type").Default("hermes"),
		field.String("investigation_id").Optional().Default(""),
		field.UUID("to_agent_id", uuid.UUID{}).Optional().Nillable(),
		field.String("to_agent_type").Optional().Default(""),
		field.String("question").NotEmpty(),
		field.String("reply").Optional().Default(""),
		field.UUID("replied_by_agent_id", uuid.UUID{}).Optional().Nillable(),
		field.String("replied_by_agent_name").Optional().Default(""),
		field.String("status").Default("pending"),
		field.Time("expires_at"),
		field.Time("created_at").Default(timeNow),
		field.Time("answered_at").Optional().Nillable(),
	}
}

func (AgentAsk) Edges() []ent.Edge {
	return nil
}

func (AgentAsk) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("to_agent_id", "status"),
		index.Fields("to_agent_type", "status"),
		index.Fields("from_agent_id", "created_at"),
		index.Fields("investigation_id"),
		index.Fields("expires_at"),
	}
}
