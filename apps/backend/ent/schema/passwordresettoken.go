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

type PasswordResetToken struct {
	ent.Schema
}

func (PasswordResetToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "password_reset_tokens"},
	}
}

func (PasswordResetToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("user_id", uuid.UUID{}),
		field.String("token_hash").Unique().NotEmpty().Sensitive(),
		field.Time("expires_at"),
		field.Bool("used").Default(false),
		field.Time("created_at").Default(timeNow),
	}
}

func (PasswordResetToken) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("password_reset_tokens").Field("user_id").Unique().Required(),
	}
}

func (PasswordResetToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("expires_at"),
	}
}
