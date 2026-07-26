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

type AgentToken struct {
	ent.Schema
}

func (AgentToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "agent_tokens"},
	}
}

func (AgentToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").NotEmpty(),
		field.Enum("agent_type").Values("hermes", "openclaw", "other").Default("hermes"),
		field.String("token_hash").Unique().NotEmpty().Sensitive(),
		field.String("lookup_prefix").NotEmpty(),
		field.Time("created_at").Default(timeNow),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable(),
		field.Bool("revoked").Default(false),
		field.Bool("enabled").Default(true),
		field.String("scope").Optional().Default(""),
		field.JSON("label_selectors", []RouteCondition{}).Optional(),
		field.Bool("default_for_investigation").Optional().Default(false),
		field.JSON("capabilities", []string{}).Default([]string{"investigate"}).Optional(),
	}
}

func (AgentToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("dm_messages", AgentDMMessage.Type),
		edge.To("ics_roles", ICSRoleAssignment.Type),
		edge.To("memories", AgentMemory.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("sent_asks", AgentAsk.Type).Annotations(entsql.Annotation{OnDelete: entsql.Restrict}),
		edge.To("received_asks", AgentAsk.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("replied_asks", AgentAsk.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
	}
}

func (AgentToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("lookup_prefix").
			Annotations(entsql.IndexWhere("revoked = false AND enabled = true")),
		index.Fields("expires_at"),
		index.Fields("default_for_investigation").
			Annotations(entsql.IndexWhere("default_for_investigation = true")),
	}
}
