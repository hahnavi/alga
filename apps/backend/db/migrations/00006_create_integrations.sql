-- +goose Up
CREATE TABLE integrations (
    id UUID PRIMARY KEY,
    mattermost_url TEXT NOT NULL DEFAULT '',
    mattermost_webhook_secret TEXT NOT NULL DEFAULT '',
    mattermost_team TEXT NOT NULL DEFAULT '',
    mattermost_default_channel TEXT NOT NULL DEFAULT '',
    mattermost_disabled BOOLEAN NOT NULL DEFAULT false,
    slack_bot_token TEXT NOT NULL DEFAULT '',
    slack_signing_secret TEXT NOT NULL DEFAULT '',
    slack_default_channel TEXT NOT NULL DEFAULT '',
    slack_disabled BOOLEAN NOT NULL DEFAULT false,
    slack_client_id TEXT NOT NULL DEFAULT '',
    slack_client_secret TEXT NOT NULL DEFAULT '',
    slack_workspace_name TEXT NOT NULL DEFAULT '',
    slack_workspace_id TEXT NOT NULL DEFAULT '',
    twilio_account_sid TEXT NOT NULL DEFAULT '',
    twilio_auth_token TEXT NOT NULL DEFAULT '',
    twilio_from_number TEXT NOT NULL DEFAULT '',
    twilio_disabled BOOLEAN NOT NULL DEFAULT false,
    telnyx_api_key TEXT NOT NULL DEFAULT '',
    telnyx_connection_id TEXT NOT NULL DEFAULT '',
    telnyx_from_number TEXT NOT NULL DEFAULT '',
    telnyx_public_key TEXT NOT NULL DEFAULT '',
    telnyx_disabled BOOLEAN NOT NULL DEFAULT false,
    telnyx_tts_voice TEXT NOT NULL DEFAULT '',
    telnyx_tts_language TEXT NOT NULL DEFAULT '',
    telnyx_tts_api_key_ref TEXT NOT NULL DEFAULT '',
    voice_provider TEXT NOT NULL DEFAULT 'twilio' CHECK (voice_provider IN ('twilio', 'telnyx')),
    hermes_platform_url TEXT NOT NULL DEFAULT '',
    hermes_platform_token TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS integrations;
