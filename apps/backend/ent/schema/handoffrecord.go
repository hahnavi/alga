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

type HandoffRecord struct {
	ent.Schema
}

func (HandoffRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "handoff_records"},
	}
}

func (HandoffRecord) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("schedule_id", uuid.UUID{}),
		field.UUID("outgoing_user_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("incoming_user_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("handoff_at"),
		field.Enum("status").Values("pending", "acknowledged").Default("pending"),
		field.Text("outgoing_notes").Optional(),
		field.Text("incoming_notes").Optional(),
		field.Time("incoming_acknowledged_at").Optional().Nillable(),
		field.Text("incident_summary").Optional(),
		field.Time("created_at").Default(timeNow).Immutable(),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (HandoffRecord) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("schedule", OnCallSchedule.Type).Field("schedule_id").Unique().Required(),
		edge.From("outgoing_user", User.Type).Ref("outgoing_handoffs").Field("outgoing_user_id").Unique(),
		edge.From("incoming_user", User.Type).Ref("incoming_handoffs").Field("incoming_user_id").Unique(),
	}
}

func (HandoffRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("schedule_id", "handoff_at"),
		index.Fields("incoming_user_id", "status"),
		index.Fields("outgoing_user_id"),
		index.Fields("status", "handoff_at"),
	}
}
