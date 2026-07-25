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
	return []ent.Edge{
		edge.From("team", Team.Type).Ref("on_call_schedule").Field("team_id").Unique(),
		edge.To("layers", ScheduleLayer.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("overrides", ScheduleOverride.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
	}
}

func (OnCallSchedule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("team_id").
			Unique().
			Annotations(entsql.IndexWhere("team_id IS NOT NULL")),
	}
}
