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

type KnowledgeNote struct {
	ent.Schema
}

func (KnowledgeNote) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "knowledge_notes"},
	}
}

func (KnowledgeNote) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.Enum("kind").Values("runbook", "known_issue", "service_owner", "fact"),
		field.String("title").NotEmpty(),
		field.String("body_markdown").NotEmpty(),
		field.JSON("tags", []string{}).Optional(),
		field.JSON("selectors", []RouteCondition{}).Optional(),
		field.UUID("author_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("author_type").Values("user", "agent").Default("user"),
		field.String("author_name").Optional().Default(""),
		field.String("source_investigation_id").Optional().Default(""),
		field.Float("confidence").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (KnowledgeNote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("author", User.Type).Ref("knowledge_notes").Field("author_id").Unique(),
	}
}

func (KnowledgeNote) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kind", "updated_at"),
		index.Fields("expires_at"),
		index.Fields("author_id"),
	}
}
