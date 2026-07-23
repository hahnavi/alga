package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// EscalationLevelRecord and EscalationTargetRecord define the collapsed shape
// of an escalation policy. They are stored as a single JSONB column on the
// escalation_policies row (no separate levels/targets tables). The structs
// double as the API/store record types so the JSON tag set drives both the
// wire format and the on-disk format.
//
// Only the level_number is the canonical key within a policy; the slice order
// itself is not significant. Callers (engine, sweep worker) should sort by
// level_number before consuming.
type EscalationLevelRecord struct {
	LevelNumber    int                      `json:"level_number"`
	DelayMinutes   int                      `json:"delay_minutes"`
	NotifyChannels []string                 `json:"notify_channels,omitempty"`
	Targets        []EscalationTargetRecord `json:"targets,omitempty"`
}

// EscalationTargetRecord captures one target inside a level. Exactly one of
// TargetUserID / TargetTeamID is set, identified by TargetType ("user",
// "team"). A "team" target resolves to whoever is currently on call for the
// team's auto-provisioned on-call schedule — paging an entire team's
// membership is no longer supported.
type EscalationTargetRecord struct {
	TargetType   string     `json:"target_type"`
	TargetUserID *uuid.UUID `json:"target_user_id,omitempty"`
	TargetTeamID *uuid.UUID `json:"target_team_id,omitempty"`
}

type EscalationPolicy struct {
	ent.Schema
}

func (EscalationPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "escalation_policies"},
	}
}

func (EscalationPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.String("name").Unique().NotEmpty(),
		field.String("description").Default(""),
		field.Int("repeat_count").Default(3),
		// Collapsed levels structure stored as JSONB.
		field.JSON("levels", []EscalationLevelRecord{}).Default([]EscalationLevelRecord{}),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (EscalationPolicy) Edges() []ent.Edge {
	return nil
}
