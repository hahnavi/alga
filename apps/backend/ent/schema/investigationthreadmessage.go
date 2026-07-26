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

type InvestigationThreadMessage struct {
	ent.Schema
}

func (InvestigationThreadMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "investigation_thread_messages"},
	}
}

func (InvestigationThreadMessage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("thread_id", uuid.UUID{}),
		field.String("type").Default("comment"),
		field.String("source").Default("user"),
		field.Text("message").NotEmpty(),
		field.Bool("internal").Default(false),
		field.Bool("edited").Default(false),
		field.String("user_id").Optional().Default(""),
		field.String("username").Optional().Default(""),
		field.String("agent_type").Optional().Default(""),
		field.String("mm_post_id").Optional().Default(""),
		field.String("slack_message_ts").Optional().Default(""),
		field.String("reply_to_message_id").Optional().Default(""),
		field.JSON("mentions", []string{}).Optional(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (InvestigationThreadMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("thread", InvestigationThread.Type).
			Ref("messages").
			Unique().
			Required().
			Field("thread_id"),
	}
}

func (InvestigationThreadMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("thread_id", "created_at"),
	}
}
