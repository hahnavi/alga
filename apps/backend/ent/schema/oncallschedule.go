package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type OnCallSchedule struct {
	ent.Schema
}

func (OnCallSchedule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "on_call_schedules"},
	}
}

// OnCallSchedule has no name/description/timezone of its own: its display name
// is derived dynamically from the associated team, and timezone lives per-layer
// (ScheduleLayer.timezone). A schedule is auto-provisioned when a team is
// created (one schedule per team).
func (OnCallSchedule) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("team_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (OnCallSchedule) Edges() []ent.Edge {
	return nil
}

func (OnCallSchedule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id"),
	}
}
