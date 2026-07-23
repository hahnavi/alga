package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type HandoffRecord struct {
	ent.Schema
}

func (HandoffRecord) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).Unique(),
		field.UUID("schedule_id", uuid.UUID{}),
		field.UUID("outgoing_user_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("incoming_user_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("handoff_at"),
		field.Enum("status").Values("pending", "acknowledged").Default("pending"),
		field.Text("outgoing_notes").Optional(),
		field.Text("incoming_notes").Optional(),
		field.Time("incoming_acknowledged_at").Optional().Nillable(),
		field.Text("incident_summary").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (HandoffRecord) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("schedule", OnCallSchedule.Type).Field("schedule_id").Unique().Required(),
	}
}

func (HandoffRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("schedule_id", "handoff_at"),
		index.Fields("incoming_user_id", "status"),
	}
}
