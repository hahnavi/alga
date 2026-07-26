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

type Heartbeat struct {
	ent.Schema
}

func (Heartbeat) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "heartbeats"},
	}
}

func (Heartbeat) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").NotEmpty(),
		field.String("description").Default(""),
		field.Int("interval_seconds").Positive(),
		field.Int("grace_seconds").Min(0).Default(60),
		field.Bool("enabled").Default(true),
		field.UUID("owner_team_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("status").Values("healthy", "expired").Default("healthy"),
		field.Enum("severity").Values("critical", "warning", "info").Default("warning"),
		field.JSON("labels", map[string]string{}).Optional(),
		field.String("ping_token_hash").Unique().NotEmpty(),
		field.String("lookup_prefix").NotEmpty(),
		field.Time("last_ping_at").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_breach_at").Optional().Nillable(),
		field.String("created_by").Optional(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (Heartbeat) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner_team", Team.Type).Ref("heartbeats").Field("owner_team_id").Unique(),
	}
}

func (Heartbeat) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "status", "expires_at"),
		index.Fields("enabled", "last_ping_at"),
		index.Fields("owner_team_id"),
		index.Fields("lookup_prefix"),
	}
}
