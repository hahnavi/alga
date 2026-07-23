package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type AuditLog struct {
	ent.Schema
}

func (AuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "audit_logs"},
	}
}

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.Time("timestamp").Default(timeNow),
		field.String("event").NotEmpty(),
		field.UUID("user_id", uuid.UUID{}).Optional().Nillable(),
		field.String("username").Optional().Default(""),
		field.String("ip").Optional().Default(""),
		field.String("user_agent").Optional().Default(""),
		field.Bool("success").Default(true),
		field.JSON("details", map[string]any{}).Optional(),
		field.String("request_id").Optional().Default(""),
	}
}

func (AuditLog) Edges() []ent.Edge {
	return nil
}

func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("timestamp"),
		index.Fields("event"),
	}
}
