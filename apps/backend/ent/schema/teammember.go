package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type TeamMember struct {
	ent.Schema
}

func (TeamMember) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "team_members"},
	}
}

func (TeamMember) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("team_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		field.String("role").Default("member"),
		field.Time("created_at").Default(timeNow),
	}
}

func (TeamMember) Edges() []ent.Edge {
	return nil
}

func (TeamMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id", "user_id").Unique(),
		index.Fields("user_id"),
	}
}
