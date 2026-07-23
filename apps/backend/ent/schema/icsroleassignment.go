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

type ICSRoleAssignment struct {
	ent.Schema
}

func (ICSRoleAssignment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "ics_role_assignments"},
	}
}

func (ICSRoleAssignment) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("role_type").Default("responder"),
		field.String("status").Default("active"),
		field.String("assignee_type").Default("user"),
		field.Text("scope_description").Optional().Nillable(),
		field.String("ended_reason").Optional().Nillable(),
		field.Time("started_at").Default(timeNow),
		field.Time("ended_at").Optional().Nillable(),
	}
}

func (ICSRoleAssignment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("incident", Incident.Type).Ref("ics_roles").Unique().Required(),
		edge.From("user", User.Type).Ref("ics_role_assignments").Unique(),
		edge.From("agent_token", AgentToken.Type).Ref("ics_roles").Unique(),
		edge.To("parent", ICSRoleAssignment.Type).Unique(),
	}
}

func (ICSRoleAssignment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role_type"),
		index.Fields("status"),
	}
}
