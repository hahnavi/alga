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
		// Singleton row; id is fixed so store lookups are deterministic.
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID {
			return uuid.UUID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		}).StorageKey("id"),
		field.String("mattermost_url").Optional().Default(""),
		// The seven fields below marked .Sensitive() hold AEAD ciphertext at
		// rest, not plaintext: store/integrations.go encrypts on Save and
		// decrypts on Get via crypto.Default(). They are marked Sensitive so
		// ciphertext never leaks into responses or logs. The column names do
		// not carry an _encrypted suffix for historical reasons; renaming is
		// tracked as a follow-up hygiene task.
		field.String("mattermost_webhook_secret").Optional().Default("").Sensitive(),
		field.String("mattermost_team").Optional().Default(""),
		field.String("mattermost_default_channel").Optional().Default(""),
		field.Bool("mattermost_disabled").Default(false),
		field.String("slack_bot_token").Optional().Default("").Sensitive(),
		field.String("slack_signing_secret").Optional().Default("").Sensitive(),
		field.String("slack_default_channel").Optional().Default(""),
		field.Bool("slack_disabled").Default(false),
		field.String("slack_client_id").Optional().Default(""),
		field.String("slack_client_secret").Optional().Default("").Sensitive(),
		field.String("slack_workspace_name").Optional().Default(""),
		field.String("slack_workspace_id").Optional().Default(""),
		field.String("twilio_account_sid").Optional().Default(""),
		field.String("twilio_auth_token").Optional().Default("").Sensitive(),
		field.String("twilio_from_number").Optional().Default(""),
		field.Bool("twilio_disabled").Default(false),
		field.String("telnyx_api_key").Optional().Default("").Sensitive(),
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
		field.Enum("voice_provider").Values("twilio", "telnyx").Optional().Default("twilio"),
		field.String("hermes_platform_url").Optional().Default(""),
		field.String("hermes_platform_token").Optional().Default("").Sensitive(),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (Integration) Edges() []ent.Edge {
	return nil
}
