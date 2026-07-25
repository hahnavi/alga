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
		edge.To("sessions", Session.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("password_reset_tokens", PasswordResetToken.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("personal_access_tokens", PersonalAccessToken.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("oidc_identities", OIDCIdentity.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("team_members", TeamMember.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("ics_role_assignments", ICSRoleAssignment.Type),
		edge.To("document_edits", IncidentDocument.Type),
		edge.To("commander_incidents", Incident.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("communicator_incidents", Incident.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("responder_incidents", Incident.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("triage_overrides", TriageResult.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("approved_post_mortems", PostMortem.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("triage_rules", TriageRule.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("knowledge_notes", KnowledgeNote.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("schedule_overrides", ScheduleOverride.Type).Annotations(entsql.Annotation{OnDelete: entsql.Cascade}),
		edge.To("outgoing_handoffs", HandoffRecord.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
		edge.To("incoming_handoffs", HandoffRecord.Type).Annotations(entsql.Annotation{OnDelete: entsql.SetNull}),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("google_id"),
		index.Fields("slack_user_id").
			Unique().
			Annotations(entsql.IndexWhere("slack_user_id <> ''")),
	}
}
