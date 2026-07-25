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

type ScheduleOverride struct {
	ent.Schema
}

func (ScheduleOverride) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "schedule_overrides"},
	}
}

func (ScheduleOverride) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("schedule_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		field.Time("start_at"),
		field.Time("end_at"),
		field.UUID("created_by", uuid.UUID{}).Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
	}
}

func (ScheduleOverride) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("schedule", OnCallSchedule.Type).Ref("overrides").Field("schedule_id").Unique().Required(),
		edge.From("user", User.Type).Ref("schedule_overrides").Field("user_id").Unique().Required(),
	}
}

func (ScheduleOverride) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("schedule_id"),
		index.Fields("user_id"),
		index.Fields("start_at", "end_at"),
	}
}
