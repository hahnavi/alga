package store

import (
	"context"
	"fmt"
	"time"

	"alga/ent"
)

type MapStringAny = map[string]any

type SystemConfigValues struct {
	CorrelationWindow                  string       `json:"correlation_window"`
	CorrelationCooldownTTL             string       `json:"correlation_cooldown_ttl"`
	InvestigationTimeout               string       `json:"investigation_timeout"`
	MaxConcurrentInvestigations        int          `json:"max_concurrent_investigations"`
	AgentPresenceTTL                   string       `json:"agent_presence_ttl"`
	AgentDisconnectGrace               string       `json:"agent_disconnect_grace"`
	SchedulerLeaderTTL                 string       `json:"scheduler_leader_ttl"`
	SessionExpiryHours                 int          `json:"session_expiry_hours"`
	LogLevel                           string       `json:"log_level"`
	OnboardingCompleted                bool         `json:"onboarding_completed"`
	SlackIncidentChannelsEnabled       bool         `json:"slack_incident_channels_enabled"`
	SlackIncidentChannelVisibility     string       `json:"slack_incident_channel_visibility"`
	SlackIncidentChannelTriggerStatus  string       `json:"slack_incident_channel_trigger_status"`
	SlackIncidentChannelArchiveOnClose bool         `json:"slack_incident_channel_archive_on_close"`
	IncidentSummaryEnabled             bool         `json:"incident_summary_enabled"`
	IncidentSummaryInterval            string       `json:"incident_summary_interval"`
	IncidentSummaryIntervals           MapStringAny `json:"incident_summary_intervals,omitempty"`

	// Authentication — Google OAuth login.
	GoogleOAuthEnabled     bool   `json:"google_oauth_enabled"`
	GoogleClientID         string `json:"google_client_id"`
	GoogleClientSecretEnc  string `json:"google_client_secret_enc,omitempty"`
	GoogleOAuthRedirectURL string `json:"google_oauth_redirect_url"`

	UpdatedAt time.Time `json:"updated_at"`
}

type SystemConfigStore interface {
	Get() (*SystemConfigValues, error)
	Save(cfg SystemConfigValues) error
}

type pgSystemConfigStore struct {
	pgStoreBase
}

func newPGSystemConfigStore(client *ent.Client) SystemConfigStore {
	return &pgSystemConfigStore{pgStoreBase{client: client}}
}

func (s *pgSystemConfigStore) Get() (*SystemConfigValues, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	r, err := s.client.SystemConfig.Get(ctx, singletonUUID())
	if err != nil {
		return handleQueryErr[*SystemConfigValues](err, "system config")
	}
	if r.Config == nil {
		return nil, nil
	}

	out := &SystemConfigValues{}
	if v, ok := r.Config["correlation_window"].(string); ok {
		out.CorrelationWindow = v
	}
	if v, ok := r.Config["correlation_cooldown_ttl"].(string); ok {
		out.CorrelationCooldownTTL = v
	}
	if v, ok := r.Config["investigation_timeout"].(string); ok {
		out.InvestigationTimeout = v
	}
	if v, ok := r.Config["max_concurrent_investigations"].(float64); ok {
		out.MaxConcurrentInvestigations = int(v)
	}
	if v, ok := r.Config["agent_presence_ttl"].(string); ok {
		out.AgentPresenceTTL = v
	}
	if v, ok := r.Config["agent_disconnect_grace"].(string); ok {
		out.AgentDisconnectGrace = v
	}
	if v, ok := r.Config["scheduler_leader_ttl"].(string); ok {
		out.SchedulerLeaderTTL = v
	}
	if v, ok := r.Config["session_expiry_hours"].(float64); ok {
		out.SessionExpiryHours = int(v)
	}
	if v, ok := r.Config["log_level"].(string); ok {
		out.LogLevel = v
	}
	if v, ok := r.Config["onboarding_completed"].(bool); ok {
		out.OnboardingCompleted = v
	}
	if v, ok := r.Config["slack_incident_channels_enabled"].(bool); ok {
		out.SlackIncidentChannelsEnabled = v
	}
	if v, ok := r.Config["slack_incident_channel_visibility"].(string); ok {
		out.SlackIncidentChannelVisibility = v
	} else {
		out.SlackIncidentChannelVisibility = "private"
	}
	if v, ok := r.Config["slack_incident_channel_trigger_status"].(string); ok {
		out.SlackIncidentChannelTriggerStatus = v
	} else {
		out.SlackIncidentChannelTriggerStatus = "active"
	}
	if v, ok := r.Config["slack_incident_channel_archive_on_close"].(bool); ok {
		out.SlackIncidentChannelArchiveOnClose = v
	} else {
		out.SlackIncidentChannelArchiveOnClose = true
	}
	if v, ok := r.Config["incident_summary_enabled"].(bool); ok {
		out.IncidentSummaryEnabled = v
	}
	if v, ok := r.Config["incident_summary_interval"].(string); ok {
		out.IncidentSummaryInterval = v
	} else {
		out.IncidentSummaryInterval = "15m"
	}
	if v, ok := r.Config["incident_summary_intervals"].(map[string]any); ok {
		out.IncidentSummaryIntervals = v
	}

	// Authentication — Google OAuth.
	if v, ok := r.Config["google_oauth_enabled"].(bool); ok {
		out.GoogleOAuthEnabled = v
	} else {
		out.GoogleOAuthEnabled = true
	}
	if v, ok := r.Config["google_client_id"].(string); ok {
		out.GoogleClientID = v
	}
	if v, ok := r.Config["google_client_secret_enc"].(string); ok {
		out.GoogleClientSecretEnc = v
	}
	if v, ok := r.Config["google_oauth_redirect_url"].(string); ok {
		out.GoogleOAuthRedirectURL = v
	}
	out.UpdatedAt = r.UpdatedAt

	return out, nil
}

func (s *pgSystemConfigStore) Save(cfg SystemConfigValues) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	data := map[string]any{
		"correlation_window":                      cfg.CorrelationWindow,
		"correlation_cooldown_ttl":                cfg.CorrelationCooldownTTL,
		"investigation_timeout":                   cfg.InvestigationTimeout,
		"max_concurrent_investigations":           cfg.MaxConcurrentInvestigations,
		"agent_presence_ttl":                      cfg.AgentPresenceTTL,
		"agent_disconnect_grace":                  cfg.AgentDisconnectGrace,
		"scheduler_leader_ttl":                    cfg.SchedulerLeaderTTL,
		"session_expiry_hours":                    cfg.SessionExpiryHours,
		"log_level":                               cfg.LogLevel,
		"onboarding_completed":                    cfg.OnboardingCompleted,
		"slack_incident_channels_enabled":         cfg.SlackIncidentChannelsEnabled,
		"slack_incident_channel_visibility":       cfg.SlackIncidentChannelVisibility,
		"slack_incident_channel_trigger_status":   cfg.SlackIncidentChannelTriggerStatus,
		"slack_incident_channel_archive_on_close": cfg.SlackIncidentChannelArchiveOnClose,
		"incident_summary_enabled":                cfg.IncidentSummaryEnabled,
		"incident_summary_interval":               cfg.IncidentSummaryInterval,
		"incident_summary_intervals":              cfg.IncidentSummaryIntervals,

		"google_oauth_enabled":      cfg.GoogleOAuthEnabled,
		"google_client_id":          cfg.GoogleClientID,
		"google_client_secret_enc":  cfg.GoogleClientSecretEnc,
		"google_oauth_redirect_url": cfg.GoogleOAuthRedirectURL,
	}

	sid := singletonUUID()
	now := time.Now().UTC()

	existing, err := s.client.SystemConfig.Get(ctx, sid)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("failed to check existing system config: %w", err)
	}

	if existing != nil {
		_, err = s.client.SystemConfig.UpdateOneID(sid).
			SetConfig(data).
			SetUpdatedAt(now).
			Save(ctx)
	} else {
		_, err = s.client.SystemConfig.Create().
			SetID(sid).
			SetConfig(data).
			SetUpdatedAt(now).
			Save(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to save system config: %w", err)
	}
	return nil
}
