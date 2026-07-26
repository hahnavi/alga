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

type AgentMemory struct {
	ent.Schema
}

func (AgentMemory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agent_memories"},
	}
}

func (AgentMemory) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("content").NotEmpty(),
		field.Enum("memory_type").Values("fact", "pattern", "procedure").Default("fact"),
		field.String("hash").Unique().NotEmpty(),
		field.JSON("embedding", []float32{}).Optional(),
		field.UUID("agent_id", uuid.UUID{}).Optional().Nillable(),
		field.String("agent_name").Optional().Default(""),
		field.String("agent_type").Optional().Default(""),
		field.String("investigation_id").Optional().Default(""),
		field.String("correlation_key").Optional().Default(""),
		field.JSON("labels", map[string]string{}).Optional(),
		field.JSON("entities", []string{}).Optional(),
		field.JSON("metadata", map[string]any{}).Optional(),
		field.Float("confidence").Optional().Nillable().Min(0).Max(1),
		field.Int("access_count").Default(0).NonNegative(),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (AgentMemory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("agent", AgentToken.Type).Ref("memories").Field("agent_id").Unique(),
	}
}

func (AgentMemory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_id", "created_at"),
		index.Fields("investigation_id"),
		index.Fields("memory_type"),
		index.Fields("expires_at"),
		index.Fields("correlation_key"),
	}
}
