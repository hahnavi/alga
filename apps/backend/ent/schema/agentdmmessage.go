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

type AgentDMMessage struct {
	ent.Schema
}

func (AgentDMMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agent_dm_messages"},
	}
}

func (AgentDMMessage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("chat_id").Default("alga_dm"),
		field.String("role").NotEmpty(),
		field.String("body").NotEmpty(),
		field.String("user_id").Optional().Nillable(),
		field.String("username").Optional().Nillable(),
		field.Bool("edited").Default(false),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
		field.UUID("agent_token_id", uuid.UUID{}),
	}
}

func (AgentDMMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agent_token", AgentToken.Type).
			Ref("dm_messages").
			Unique().
			Required().
			Field("agent_token_id"),
	}
}

func (AgentDMMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("chat_id", "created_at"),
		index.Fields("agent_token_id"),
	}
}
