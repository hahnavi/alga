package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"alga/config"
	algacrypto "alga/crypto"
	"alga/db/models"
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

func newPGIntegrationStore(db *bun.DB) IntegrationStore {
	return &pgIntegrationStore{pgStoreBase{db: db}}
}

func (s *pgIntegrationStore) Get() (*IntegrationConfig, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	cfg := new(models.Integration)
	err := s.db.NewSelect().Model(cfg).Where("id = ?", singletonUUID()).Scan(ctx)
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
		TelnyxTTSVoice:           cfg.TelnyxTTSVoice,
		TelnyxTTSLanguage:        cfg.TelnyxTTSLanguage,
		TelnyxTTSAPIKeyRef:       cfg.TelnyxTTSAPIKeyRef,
		VoiceProvider:            config.NormalizeVoiceProvider(cfg.VoiceProvider),
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

	existing := new(models.Integration)
	err := s.db.NewSelect().Model(existing).Where("id = ?", sid).Scan(ctx)
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("failed to check existing integration: %w", err)
	}

	voiceProvider := config.NormalizeVoiceProvider(encrypted.VoiceProvider)

	if err == nil {
		_, err = s.db.NewUpdate().Model((*models.Integration)(nil)).
			Set("mattermost_url = ?", encrypted.MattermostURL).
			Set("mattermost_webhook_secret = ?", encrypted.MattermostWebhookSecret).
			Set("mattermost_team = ?", encrypted.MattermostTeam).
			Set("mattermost_default_channel = ?", encrypted.MattermostDefaultChannel).
			Set("mattermost_disabled = ?", encrypted.MattermostDisabled).
			Set("slack_bot_token = ?", encrypted.SlackBotToken).
			Set("slack_signing_secret = ?", encrypted.SlackSigningSecret).
			Set("slack_default_channel = ?", encrypted.SlackDefaultChannel).
			Set("slack_disabled = ?", encrypted.SlackDisabled).
			Set("slack_client_id = ?", encrypted.SlackClientID).
			Set("slack_client_secret = ?", encrypted.SlackClientSecret).
			Set("slack_workspace_name = ?", encrypted.SlackWorkspaceName).
			Set("slack_workspace_id = ?", encrypted.SlackWorkspaceID).
			Set("twilio_account_sid = ?", encrypted.TwilioAccountSID).
			Set("twilio_auth_token = ?", encrypted.TwilioAuthToken).
			Set("twilio_from_number = ?", encrypted.TwilioFromNumber).
			Set("twilio_disabled = ?", encrypted.TwilioDisabled).
			Set("telnyx_api_key = ?", encrypted.TelnyxAPIKey).
			Set("telnyx_connection_id = ?", encrypted.TelnyxConnectionID).
			Set("telnyx_from_number = ?", encrypted.TelnyxFromNumber).
			Set("telnyx_public_key = ?", encrypted.TelnyxPublicKey).
			Set("telnyx_disabled = ?", encrypted.TelnyxDisabled).
			Set("telnyx_tts_voice = ?", encrypted.TelnyxTTSVoice).
			Set("telnyx_tts_language = ?", encrypted.TelnyxTTSLanguage).
			Set("telnyx_tts_api_key_ref = ?", encrypted.TelnyxTTSAPIKeyRef).
			Set("voice_provider = ?", voiceProvider).
			Set("hermes_platform_url = ?", encrypted.HermesPlatformURL).
			Set("hermes_platform_token = ?", encrypted.HermesPlatformToken).
			Set("updated_at = ?", encrypted.UpdatedAt).
			Where("id = ?", sid).
			Exec(ctx)
	} else {
		m := &models.Integration{
			ID:                       sid,
			MattermostURL:            encrypted.MattermostURL,
			MattermostWebhookSecret:  encrypted.MattermostWebhookSecret,
			MattermostTeam:           encrypted.MattermostTeam,
			MattermostDefaultChannel: encrypted.MattermostDefaultChannel,
			MattermostDisabled:       encrypted.MattermostDisabled,
			SlackBotToken:            encrypted.SlackBotToken,
			SlackSigningSecret:       encrypted.SlackSigningSecret,
			SlackDefaultChannel:      encrypted.SlackDefaultChannel,
			SlackDisabled:            encrypted.SlackDisabled,
			SlackClientID:            encrypted.SlackClientID,
			SlackClientSecret:        encrypted.SlackClientSecret,
			SlackWorkspaceName:       encrypted.SlackWorkspaceName,
			SlackWorkspaceID:         encrypted.SlackWorkspaceID,
			TwilioAccountSid:         encrypted.TwilioAccountSID,
			TwilioAuthToken:          encrypted.TwilioAuthToken,
			TwilioFromNumber:         encrypted.TwilioFromNumber,
			TwilioDisabled:           encrypted.TwilioDisabled,
			TelnyxAPIKey:             encrypted.TelnyxAPIKey,
			TelnyxConnectionID:       encrypted.TelnyxConnectionID,
			TelnyxFromNumber:         encrypted.TelnyxFromNumber,
			TelnyxPublicKey:          encrypted.TelnyxPublicKey,
			TelnyxDisabled:           encrypted.TelnyxDisabled,
			TelnyxTTSVoice:           encrypted.TelnyxTTSVoice,
			TelnyxTTSLanguage:        encrypted.TelnyxTTSLanguage,
			TelnyxTTSAPIKeyRef:       encrypted.TelnyxTTSAPIKeyRef,
			VoiceProvider:            voiceProvider,
			HermesPlatformURL:        encrypted.HermesPlatformURL,
			HermesPlatformToken:      encrypted.HermesPlatformToken,
			UpdatedAt:                encrypted.UpdatedAt,
		}
		_, err = s.db.NewInsert().Model(m).Exec(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to save integration config: %w", err)
	}
	return nil
}
