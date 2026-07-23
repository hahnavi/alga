package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
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
	}
}

func (AgentDMMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agent_token", AgentToken.Type).
			Ref("dm_messages").
			Unique().
			Required(),
	}
}

func (AgentDMMessage) Indexes() []ent.Index {
	return []ent.Index{}
}
