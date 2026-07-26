package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Team struct {
	ent.Schema
}

func (Team) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "teams"},
	}
}

func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").Unique().NotEmpty(),
		field.String("description").Default(""),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (Team) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("team_members", TeamMember.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("owned_services", Service.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("owned_status_pages", StatusPage.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("on_call_schedule", OnCallSchedule.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("heartbeats", Heartbeat.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
	}
}
