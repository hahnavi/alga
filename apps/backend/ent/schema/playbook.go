package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Playbook struct {
	ent.Schema
}

func (Playbook) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).Unique(),
		field.String("title").Unique(),
		field.Enum("kind").Values("procedure", "mitigation"),
		field.Text("summary").Optional(),
		field.UUID("service_id", uuid.UUID{}).Optional().Nillable(),
		field.JSON("label_selectors", []map[string]any{}).Optional(),
		field.JSON("tags", []string{}).Optional(),
		field.UUID("created_by", uuid.UUID{}),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Playbook) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("service", Service.Type).Field("service_id").Unique(),
		edge.To("created_by_user", User.Type).Field("created_by").Unique().Required(),
		edge.To("steps", PlaybookStep.Type),
	}
}
