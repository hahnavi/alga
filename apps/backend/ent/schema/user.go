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

type User struct {
	ent.Schema
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "users"},
	}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("email").Unique().NotEmpty(),
		field.String("password").Sensitive().NotEmpty(),
		field.String("role").Default("viewer"),
		field.String("full_name").Optional().Default(""),
		field.String("phone").Optional().Default(""),
		field.String("phone_country").Optional().Default(""),
		field.Int("failed_login_attempts").Default(0).NonNegative(),
		field.Time("locked_until").Optional().Nillable(),
		field.Time("last_failed_login").Optional().Nillable(),
		field.Time("last_login_at").Optional().Nillable(),
		field.String("last_login_ip").Optional().Default(""),
		field.String("google_id").Optional().Default(""),
		field.String("slack_user_id").Optional().Default(""),
		field.String("slack_display_name").Optional().Default(""),
		field.JSON("notification_preferences", map[string]any{}).Optional(),
		field.Bool("voice_opt_out").Default(false),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ics_role_assignments", ICSRoleAssignment.Type),
		edge.To("document_edits", IncidentDocument.Type),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("google_id"),
	}
}
