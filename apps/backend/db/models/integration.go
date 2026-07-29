package models

import (
	"time"

	"github.com/google/uuid"
)

type Integration struct {
	ID                       uuid.UUID `bun:"id,pk"`
	MattermostURL            string    `bun:"mattermost_url,default:''"`
	MattermostWebhookSecret  string    `bun:"mattermost_webhook_secret,default:''"`
	MattermostTeam           string    `bun:"mattermost_team,default:''"`
	MattermostDefaultChannel string    `bun:"mattermost_default_channel,default:''"`
	MattermostDisabled       bool      `bun:"mattermost_disabled,notnull,default:false"`
	SlackBotToken            string    `bun:"slack_bot_token,default:''"`
	SlackSigningSecret       string    `bun:"slack_signing_secret,default:''"`
	SlackDefaultChannel      string    `bun:"slack_default_channel,default:''"`
	SlackDisabled            bool      `bun:"slack_disabled,notnull,default:false"`
	SlackClientID            string    `bun:"slack_client_id,default:''"`
	SlackClientSecret        string    `bun:"slack_client_secret,default:''"`
	SlackWorkspaceName       string    `bun:"slack_workspace_name,default:''"`
	SlackWorkspaceID         string    `bun:"slack_workspace_id,default:''"`
	TwilioAccountSid         string    `bun:"twilio_account_sid,default:''"`
	TwilioAuthToken          string    `bun:"twilio_auth_token,default:''"`
	TwilioFromNumber         string    `bun:"twilio_from_number,default:''"`
	TwilioDisabled           bool      `bun:"twilio_disabled,notnull,default:false"`
	TelnyxAPIKey             string    `bun:"telnyx_api_key,default:''"`
	TelnyxConnectionID       string    `bun:"telnyx_connection_id,default:''"`
	TelnyxFromNumber         string    `bun:"telnyx_from_number,default:''"`
	TelnyxPublicKey          string    `bun:"telnyx_public_key,default:''"`
	TelnyxDisabled           bool      `bun:"telnyx_disabled,notnull,default:false"`
	TelnyxTTSVoice           string    `bun:"telnyx_tts_voice,default:''"`
	TelnyxTTSLanguage        string    `bun:"telnyx_tts_language,default:''"`
	TelnyxTTSAPIKeyRef       string    `bun:"telnyx_tts_api_key_ref,default:''"`
	VoiceProvider            string    `bun:"voice_provider,default:'twilio'"`
	HermesPlatformURL        string    `bun:"hermes_platform_url,default:''"`
	HermesPlatformToken      string    `bun:"hermes_platform_token,default:''"`
	UpdatedAt                time.Time `bun:"updated_at,notnull,default:current_timestamp"`
}

func (*Integration) TableName() string { return "integrations" }
