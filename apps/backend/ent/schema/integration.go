package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Integration struct {
	ent.Schema
}

func (Integration) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "integrations"},
	}
}

func (Integration) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).StorageKey("id").Default(func() uuid.UUID {
			return uuid.UUID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		}),
		field.String("mattermost_url").Optional().Default(""),
		field.String("mattermost_webhook_secret").Optional().Default(""),
		field.String("mattermost_team").Optional().Default(""),
		field.String("mattermost_default_channel").Optional().Default(""),
		field.Bool("mattermost_disabled").Default(false),
		field.String("slack_bot_token").Optional().Default(""),
		field.String("slack_signing_secret").Optional().Default(""),
		field.String("slack_default_channel").Optional().Default(""),
		field.Bool("slack_disabled").Default(false),
		field.String("slack_client_id").Optional().Default(""),
		field.String("slack_client_secret").Optional().Default(""),
		field.String("slack_workspace_name").Optional().Default(""),
		field.String("slack_workspace_id").Optional().Default(""),
		field.String("twilio_account_sid").Optional().Default(""),
		field.String("twilio_auth_token").Optional().Default(""),
		field.String("twilio_from_number").Optional().Default(""),
		field.Bool("twilio_disabled").Default(false),
		field.String("telnyx_api_key").Optional().Default(""),
		field.String("telnyx_connection_id").Optional().Default(""),
		field.String("telnyx_from_number").Optional().Default(""),
		field.String("telnyx_public_key").Optional().Default(""),
		field.Bool("telnyx_disabled").Default(false),
		// telnyx_tts_voice carries the full Telnyx voice identifier including
		// provider prefix (e.g. "ElevenLabs.eleven_flash_v2_5.<id>" or
		// "Polly.Brian"); the provider is encoded in the prefix, so there is no
		// separate service field. See Telnyx TTS docs.
		field.String("telnyx_tts_voice").Optional().Default(""),
		// telnyx_tts_language is an optional BCP-47 language hint passed
		// through to the speak payload; ignored by some providers (e.g. Polly).
		field.String("telnyx_tts_language").Optional().Default(""),
		// telnyx_tts_api_key_ref references a Telnyx integration secret that
		// holds the ElevenLabs (or other BYOK) API key. Required when the voice
		// prefix is "ElevenLabs."; otherwise ignored. Stored plaintext because
		// it is an identifier, not the secret itself.
		field.String("telnyx_tts_api_key_ref").Optional().Default(""),
		field.String("voice_provider").Optional().Default("twilio"),
		field.String("hermes_platform_url").Optional().Default(""),
		field.String("hermes_platform_token").Optional().Default(""),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (Integration) Edges() []ent.Edge {
	return nil
}
