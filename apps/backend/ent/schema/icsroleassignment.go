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
		field.UUID("parent_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("incident_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("agent_token_id", uuid.UUID{}).Optional().Nillable(),
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
		edge.From("incident", Incident.Type).Ref("ics_roles").Unique().Required().Field("incident_id"),
		edge.From("user", User.Type).Ref("ics_role_assignments").Unique().Field("user_id"),
		edge.From("agent_token", AgentToken.Type).Ref("ics_roles").Unique().Field("agent_token_id"),
		edge.To("children", ICSRoleAssignment.Type),
		edge.From("parent", ICSRoleAssignment.Type).Ref("children").Field("parent_id").Unique(),
	}
}

func (ICSRoleAssignment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role_type"),
		index.Fields("status"),
		index.Fields("parent_id"),
		index.Fields("incident_id"),
		index.Fields("user_id"),
		index.Fields("agent_token_id"),
		index.Fields("incident_id", "role_type").
			Unique().
			Annotations(entsql.IndexWhere("status = 'active'")),
	}
}
