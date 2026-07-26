package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type NotificationDeliveryLog struct {
	ent.Schema
}

func (NotificationDeliveryLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "notification_delivery_logs"},
	}
}

func (NotificationDeliveryLog) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("user_id", uuid.UUID{}),
		field.UUID("incident_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("notification_type").Values("escalation", "oncall_handoff", "post_mortem_review_requested", "action_item_assigned", "mention", "info"),
		field.Enum("channel").Values("email", "mattermost", "slack", "voice"),
		field.Enum("status").Values("sent", "delivered", "failed", "queued", "skipped", "skipped_no_slack_id", "skipped_no_phone", "skipped_opt_out", "skipped_dedup").Default("sent"),
		field.Text("error_message").Optional().Default(""),
		field.Time("created_at").Default(timeNow),
	}
}

func (NotificationDeliveryLog) Edges() []ent.Edge {
	return nil
}

func (NotificationDeliveryLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("incident_id"),
		index.Fields("status"),
	}
}
