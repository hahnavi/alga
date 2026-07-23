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

type IncidentCoordinationMessage struct {
	ent.Schema
}

func (IncidentCoordinationMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "incident_coordination_messages"}}
}

func (IncidentCoordinationMessage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("kind").Default("chat"),
		field.String("actor_type").Default("system"),
		field.UUID("actor_id", uuid.UUID{}).Optional().Nillable(),
		field.String("actor_display_name").Optional().Default(""),
		field.String("body").NotEmpty(),
		field.Bool("internal").Default(false),
		field.String("source").Default("alga"),
		field.String("slack_channel_id").Optional().Default(""),
		field.String("slack_message_ts").Optional().Default(""),
		field.String("slack_thread_ts").Optional().Default(""),
		field.String("provider_message_id").Optional().Default(""),
		field.String("linked_investigation_id").Optional().Default(""),
		field.UUID("parent_message_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("linked_coordination_task_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("metadata", map[string]any{}).Default(map[string]any{}),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (IncidentCoordinationMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("incident", Incident.Type).Ref("coordination_messages").Unique().Required(),
		edge.To("replies", IncidentCoordinationMessage.Type),
		edge.From("parent_message", IncidentCoordinationMessage.Type).
			Ref("replies").
			Unique().
			Field("parent_message_id"),
	}
}

func (IncidentCoordinationMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("provider_message_id"),
		index.Fields("slack_channel_id", "slack_message_ts"),
		index.Fields("parent_message_id"),
		index.Fields("linked_coordination_task_id"),
	}
}
