package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alga/config"
	"alga/ent"
	"alga/ent/integration"

	algacrypto "alga/crypto"
)

type IntegrationConfig struct {
	MattermostURL            string    `json:"mattermost_url"`
	MattermostWebhookSecret  string    `json:"mattermost_webhook_secret"`
	MattermostTeam           string    `json:"mattermost_team"`
	MattermostDefaultChannel string    `json:"mattermost_default_channel"`
	MattermostDisabled       bool      `json:"mattermost_disabled"`
	SlackBotToken            string    `json:"slack_bot_token"`
	SlackSigningSecret       string    `json:"slack_signing_secret"`
	SlackDefaultChannel      string    `json:"slack_default_channel"`
	SlackDisabled            bool      `json:"slack_disabled"`
	SlackClientID            string    `json:"slack_client_id"`
	SlackClientSecret        string    `json:"slack_client_secret"`
	SlackWorkspaceName       string    `json:"slack_workspace_name"`
	SlackWorkspaceID         string    `json:"slack_workspace_id"`
	TwilioAccountSID         string    `json:"twilio_account_sid"`
	TwilioAuthToken          string    `json:"twilio_auth_token"`
	TwilioFromNumber         string    `json:"twilio_from_number"`
	TwilioDisabled           bool      `json:"twilio_disabled"`
	TelnyxAPIKey             string    `json:"telnyx_api_key"`
	TelnyxConnectionID       string    `json:"telnyx_connection_id"`
	TelnyxFromNumber         string    `json:"telnyx_from_number"`
	TelnyxPublicKey          string    `json:"telnyx_public_key"`
	TelnyxDisabled           bool      `json:"telnyx_disabled"`
	TelnyxTTSVoice           string    `json:"telnyx_tts_voice"`
	TelnyxTTSLanguage        string    `json:"telnyx_tts_language"`
	TelnyxTTSAPIKeyRef       string    `json:"telnyx_tts_api_key_ref"`
	VoiceProvider            string    `json:"voice_provider"`
	HermesPlatformURL        string    `json:"hermes_platform_url"`
	HermesPlatformToken      string    `json:"hermes_platform_token"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type IntegrationStore interface {
	Get() (*IntegrationConfig, error)
	Save(cfg IntegrationConfig) error
}

func (c *IntegrationConfig) secretFields() []*string {
	return []*string{
		&c.MattermostWebhookSecret,
		&c.SlackBotToken,
		&c.SlackSigningSecret,
		&c.SlackClientSecret,
		&c.TwilioAuthToken,
		&c.TelnyxAPIKey,
		&c.HermesPlatformToken,
	}
}

func (c *IntegrationConfig) encryptSecrets() error {
	k := algacrypto.Default()
	if !k.Enabled() {
		return errors.New("integrations: cannot persist secrets: ENCRYPTION_KEYS is not configured")
	}
	for _, fp := range c.secretFields() {
		if *fp == "" {
			continue
		}
		ct, err := k.EncryptString(*fp)
		if err != nil {
			return fmt.Errorf("integrations: encrypt: %w", err)
		}
		*fp = ct
	}
	return nil
}

func (c *IntegrationConfig) decryptSecrets() error {
	k := algacrypto.Default()
	for _, fp := range c.secretFields() {
		if *fp == "" {
			continue
		}
		pt, err := k.DecryptString(*fp)
		if err != nil {
			return fmt.Errorf("integrations: decrypt: %w", err)
		}
		*fp = pt
	}
	return nil
}

type pgIntegrationStore struct {
	pgStoreBase
}

func newPGIntegrationStore(client *ent.Client) IntegrationStore {
	return &pgIntegrationStore{pgStoreBase{client: client}}
}

func (s *pgIntegrationStore) Get() (*IntegrationConfig, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	cfg, err := s.client.Integration.Get(ctx, singletonUUID())
	if err != nil {
		return handleQueryErr[*IntegrationConfig](err, "integration config")
	}

	out := &IntegrationConfig{
		MattermostURL:            cfg.MattermostURL,
		MattermostWebhookSecret:  cfg.MattermostWebhookSecret,
		MattermostTeam:           cfg.MattermostTeam,
		MattermostDefaultChannel: cfg.MattermostDefaultChannel,
		MattermostDisabled:       cfg.MattermostDisabled,
		SlackBotToken:            cfg.SlackBotToken,
		SlackSigningSecret:       cfg.SlackSigningSecret,
		SlackDefaultChannel:      cfg.SlackDefaultChannel,
		SlackDisabled:            cfg.SlackDisabled,
		SlackClientID:            cfg.SlackClientID,
		SlackClientSecret:        cfg.SlackClientSecret,
		SlackWorkspaceName:       cfg.SlackWorkspaceName,
		SlackWorkspaceID:         cfg.SlackWorkspaceID,
		TwilioAccountSID:         cfg.TwilioAccountSid,
		TwilioAuthToken:          cfg.TwilioAuthToken,
		TwilioFromNumber:         cfg.TwilioFromNumber,
		TwilioDisabled:           cfg.TwilioDisabled,
		TelnyxAPIKey:             cfg.TelnyxAPIKey,
		TelnyxConnectionID:       cfg.TelnyxConnectionID,
		TelnyxFromNumber:         cfg.TelnyxFromNumber,
		TelnyxPublicKey:          cfg.TelnyxPublicKey,
		TelnyxDisabled:           cfg.TelnyxDisabled,
		TelnyxTTSVoice:           cfg.TelnyxTtsVoice,
		TelnyxTTSLanguage:        cfg.TelnyxTtsLanguage,
		TelnyxTTSAPIKeyRef:       cfg.TelnyxTtsAPIKeyRef,
		VoiceProvider:            config.NormalizeVoiceProvider(string(cfg.VoiceProvider)),
		HermesPlatformURL:        cfg.HermesPlatformURL,
		HermesPlatformToken:      cfg.HermesPlatformToken,
		UpdatedAt:                cfg.UpdatedAt,
	}

	if err := out.decryptSecrets(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *pgIntegrationStore) Save(cfg IntegrationConfig) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	encrypted := cfg
	if err := encrypted.encryptSecrets(); err != nil {
		return err
	}
	encrypted.UpdatedAt = time.Now().UTC()

	sid := singletonUUID()

	existing, err := s.client.Integration.Get(ctx, sid)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("failed to check existing integration: %w", err)
	}

	if existing != nil {
		_, err = s.client.Integration.UpdateOneID(sid).
			SetMattermostURL(encrypted.MattermostURL).
			SetMattermostWebhookSecret(encrypted.MattermostWebhookSecret).
			SetMattermostTeam(encrypted.MattermostTeam).
			SetMattermostDefaultChannel(encrypted.MattermostDefaultChannel).
			SetMattermostDisabled(encrypted.MattermostDisabled).
			SetSlackBotToken(encrypted.SlackBotToken).
			SetSlackSigningSecret(encrypted.SlackSigningSecret).
			SetSlackDefaultChannel(encrypted.SlackDefaultChannel).
			SetSlackDisabled(encrypted.SlackDisabled).
			SetSlackClientID(encrypted.SlackClientID).
			SetSlackClientSecret(encrypted.SlackClientSecret).
			SetSlackWorkspaceName(encrypted.SlackWorkspaceName).
			SetSlackWorkspaceID(encrypted.SlackWorkspaceID).
			SetTwilioAccountSid(encrypted.TwilioAccountSID).
			SetTwilioAuthToken(encrypted.TwilioAuthToken).
			SetTwilioFromNumber(encrypted.TwilioFromNumber).
			SetTwilioDisabled(encrypted.TwilioDisabled).
			SetTelnyxAPIKey(encrypted.TelnyxAPIKey).
			SetTelnyxConnectionID(encrypted.TelnyxConnectionID).
			SetTelnyxFromNumber(encrypted.TelnyxFromNumber).
			SetTelnyxPublicKey(encrypted.TelnyxPublicKey).
			SetTelnyxDisabled(encrypted.TelnyxDisabled).
			SetTelnyxTtsVoice(encrypted.TelnyxTTSVoice).
			SetTelnyxTtsLanguage(encrypted.TelnyxTTSLanguage).
			SetTelnyxTtsAPIKeyRef(encrypted.TelnyxTTSAPIKeyRef).
			SetVoiceProvider(integration.VoiceProvider(config.NormalizeVoiceProvider(encrypted.VoiceProvider))).
			SetHermesPlatformURL(encrypted.HermesPlatformURL).
			SetHermesPlatformToken(encrypted.HermesPlatformToken).
			SetUpdatedAt(encrypted.UpdatedAt).
			Save(ctx)
	} else {
		_, err = s.client.Integration.Create().
			SetID(sid).
			SetMattermostURL(encrypted.MattermostURL).
			SetMattermostWebhookSecret(encrypted.MattermostWebhookSecret).
			SetMattermostTeam(encrypted.MattermostTeam).
			SetMattermostDefaultChannel(encrypted.MattermostDefaultChannel).
			SetMattermostDisabled(encrypted.MattermostDisabled).
			SetSlackBotToken(encrypted.SlackBotToken).
			SetSlackSigningSecret(encrypted.SlackSigningSecret).
			SetSlackDefaultChannel(encrypted.SlackDefaultChannel).
			SetSlackDisabled(encrypted.SlackDisabled).
			SetSlackClientID(encrypted.SlackClientID).
			SetSlackClientSecret(encrypted.SlackClientSecret).
			SetSlackWorkspaceName(encrypted.SlackWorkspaceName).
			SetSlackWorkspaceID(encrypted.SlackWorkspaceID).
			SetTwilioAccountSid(encrypted.TwilioAccountSID).
			SetTwilioAuthToken(encrypted.TwilioAuthToken).
			SetTwilioFromNumber(encrypted.TwilioFromNumber).
			SetTwilioDisabled(encrypted.TwilioDisabled).
			SetTelnyxAPIKey(encrypted.TelnyxAPIKey).
			SetTelnyxConnectionID(encrypted.TelnyxConnectionID).
			SetTelnyxFromNumber(encrypted.TelnyxFromNumber).
			SetTelnyxPublicKey(encrypted.TelnyxPublicKey).
			SetTelnyxDisabled(encrypted.TelnyxDisabled).
			SetTelnyxTtsVoice(encrypted.TelnyxTTSVoice).
			SetTelnyxTtsLanguage(encrypted.TelnyxTTSLanguage).
			SetTelnyxTtsAPIKeyRef(encrypted.TelnyxTTSAPIKeyRef).
			SetVoiceProvider(integration.VoiceProvider(config.NormalizeVoiceProvider(encrypted.VoiceProvider))).
			SetHermesPlatformURL(encrypted.HermesPlatformURL).
			SetHermesPlatformToken(encrypted.HermesPlatformToken).
			SetUpdatedAt(encrypted.UpdatedAt).
			Save(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to save integration config: %w", err)
	}
	return nil
}

func singletonUUID() [16]byte {
	return [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
}
