package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ScheduleLayer struct {
	ent.Schema
}

func (ScheduleLayer) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "schedule_layers"},
	}
}

func (ScheduleLayer) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("schedule_id", uuid.UUID{}),
		field.String("name").Default(""),
		field.String("rotation_type").Default("weekly"),
		field.Int("rotation_interval").Default(1),
		field.Time("start_date").Default(timeNow),
		field.Time("end_date").Optional().Nillable(),
		// timezone is the IANA timezone in which this layer's daily-active
		// window (below) and days_of_week are interpreted. The resolver applies
		// each layer's own timezone, so different rotations can run in different
		// timezones. Defaults to UTC.
		field.String("timezone").Default("UTC"),
		// Explicit daily-active-window restriction fields. These replace the
		// former free-form "restrictions" JSON. start_time/end_time are "HH:MM"
		// in the layer's timezone. An empty end_time means active all day.
		// days_of_week is empty when the layer is active every day.
		field.String("start_time").Default("00:00"),
		field.String("end_time").Default(""),
		field.JSON("days_of_week", []string{}).Default([]string{}),
		field.Int("priority").Default(0),
		field.JSON("user_ids", []string{}).Default([]string{}),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (ScheduleLayer) Edges() []ent.Edge {
	return nil
}

func (ScheduleLayer) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("schedule_id"),
	}
}
