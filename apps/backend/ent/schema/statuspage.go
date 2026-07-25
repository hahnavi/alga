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

type StatusPage struct {
	ent.Schema
}

func (StatusPage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "status_pages"},
	}
}

func (StatusPage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").NotEmpty(),
		field.String("slug").Unique().NotEmpty(),
		field.String("description").Default(""),
		field.String("visibility").Default("internal"),
		field.Bool("enabled").Default(true),
		field.UUID("owner_team_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (StatusPage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("components", StatusPageComponent.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.From("owner_team", Team.Type).Ref("owned_status_pages").Field("owner_team_id").Unique(),
	}
}

func (StatusPage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("owner_team_id"),
	}
}

type StatusPageComponent struct {
	ent.Schema
}

func (StatusPageComponent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "status_page_components"},
	}
}

func (StatusPageComponent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("status_page_id", uuid.UUID{}),
		field.String("name").NotEmpty(),
		field.String("description").Default(""),
		field.UUID("service_id", uuid.UUID{}).Optional().Nillable(),
		field.Int("display_order").Default(0).NonNegative(),
		field.String("status").Default("operational"),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (StatusPageComponent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("status_page", StatusPage.Type).Ref("components").Field("status_page_id").Unique().Required(),
		edge.From("service", Service.Type).Ref("status_page_components").Field("service_id").Unique(),
	}
}

func (StatusPageComponent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status_page_id", "display_order"),
		index.Fields("service_id"),
	}
}
