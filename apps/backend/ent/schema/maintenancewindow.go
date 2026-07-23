package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

type MaintenanceWindow struct {
	ent.Schema
}

func (MaintenanceWindow) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "maintenance_windows"},
	}
}

func (MaintenanceWindow) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").NotEmpty(),
		field.Time("start_time"),
		field.Time("end_time"),
		field.JSON("label_matchers", map[string]string{}).Optional(),
		field.String("created_by").Optional(),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (MaintenanceWindow) Edges() []ent.Edge {
	return nil
}

func (MaintenanceWindow) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "start_time", "end_time"),
	}
}
