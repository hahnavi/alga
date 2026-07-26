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

// CoordinationTask models an Alga-managed "subagent" work unit: a typed,
// persisted, server-gated parent/child relationship between roles.
// The commander dispatches tasks; responders/communicators execute them.
type CoordinationTask struct {
	ent.Schema
}

func (CoordinationTask) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "coordination_tasks"}}
}

func (CoordinationTask) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("incident_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("parent_task_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("kind").Values("investigate", "communicate", "verify", "mitigate", "synthesize").Default("investigate"),
		field.Enum("assignee_role").Values("commander", "communicator", "responder").Default("responder"),
		field.String("assignee_agent_id").Optional().Default(""),
		field.String("assignee_agent_name").Optional().Default(""),
		field.String("goal").NotEmpty(),
		field.JSON("input_context", map[string]any{}).Default(map[string]any{}),
		field.JSON("result", map[string]any{}).Optional(),
		field.JSON("result_schema", map[string]any{}).Optional(),
		field.UUID("linked_investigation_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("status").Values("pending", "assigned", "in_progress", "complete", "failed", "cancelled").Default("pending"),
		field.Int("priority").Default(0).NonNegative(),
		field.Time("due_at").Optional().Nillable(),
		field.Time("claimed_at").Optional().Nillable(),
		field.Time("completed_at").Optional().Nillable(),
		field.String("created_by_agent_id").Optional().Default(""),
		field.String("created_by_name").Optional().Default(""),
		field.String("failure_reason").Optional().Default(""),
		field.Int("dispatch_attempts").Default(0).NonNegative(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (CoordinationTask) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("incident", Incident.Type).
			Ref("coordination_tasks").
			Unique().
			Field("incident_id"),
		edge.To("child_tasks", CoordinationTask.Type),
		edge.From("parent_task", CoordinationTask.Type).
			Ref("child_tasks").
			Unique().
			Field("parent_task_id"),
		edge.From("linked_investigation", IncidentInvestigation.Type).Ref("linked_coordination_tasks").Field("linked_investigation_id").Unique(),
	}
}

func (CoordinationTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("incident_id"),
		index.Fields("status", "priority", "created_at"),
		index.Fields("assignee_agent_id", "status"),
		index.Fields("parent_task_id"),
		index.Fields("incident_id", "status"),
		index.Fields("assignee_role", "status"),
		index.Fields("due_at"),
		index.Fields("linked_investigation_id"),
	}
}
