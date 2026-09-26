package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	algacrypto "alga/crypto"
	"alga/logger"
	"alga/rbac"
	"alga/store"
)

func (s *Server) handleSystemConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSystemConfig(w, r)
	case http.MethodPut:
		s.handlePutSystemConfig(w, r)
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.SystemConfigRead) {
		return
	}

	s.mu.RLock()
	cfg := s.cfg
	updatedAt := s.systemConfigUpdatedAt
	s.mu.RUnlock()

	resp := map[string]any{
		"correlation_window":                      durationStr(cfg.CorrelationWindow),
		"correlation_cooldown_ttl":                durationStr(cfg.CorrelationCooldownTTL),
		"investigation_timeout":                   durationStr(cfg.InvestigationTimeout),
		"max_concurrent_investigations":           cfg.MaxConcurrentInvestigations,
		"agent_presence_ttl":                      durationStr(cfg.AgentPresenceTTL),
		"agent_disconnect_grace":                  durationStr(cfg.AgentDisconnectGrace),
		"scheduler_leader_ttl":                    durationStr(cfg.SchedulerLeaderTTL),
		"session_expiry_hours":                    cfg.SessionExpiryHrs,
		"log_level":                               cfg.LogLevel,
		"environment":                             cfg.Environment,
		"slack_incident_channels_enabled":         cfg.SlackIncidentChannelsEnabled,
		"slack_incident_channel_visibility":       cfg.SlackIncidentChannelVisibility,
		"slack_incident_channel_trigger_status":   cfg.SlackIncidentChannelTriggerStatus,
		"slack_incident_channel_archive_on_close": cfg.SlackIncidentChannelArchiveOnClose,
		"incident_summary_enabled":                cfg.IncidentSummaryEnabled,
		"incident_summary_interval":               durationStr(cfg.IncidentSummaryInterval),
		"incident_summary_intervals":              severityIntervalsToStrings(cfg.IncidentSummaryIntervals),

		// Authentication — secrets are never returned; only a "set" flag.
		"google_oauth_enabled":      cfg.GoogleOAuthEnabled,
		"google_client_id":          cfg.GoogleClientID,
		"google_client_secret_set":  cfg.GoogleClientSecret != "",
		"google_oauth_redirect_url": cfg.GoogleOAuthRedirectURL,
	}
	if !updatedAt.IsZero() {
		resp["updated_at"] = updatedAt.UTC().Format(time.RFC3339)
	}
	writeData(w, http.StatusOK, resp)
}

// systemConfigUpdate is the typed patch parsed from a PUT /system/config
// body. Nil fields were not provided and are left unchanged.
type systemConfigUpdate struct {
	LogLevel                           *string
	SessionExpiryHrs                   *int
	MaxConcurrentInvestigations        *int
	CorrelationWindow                  *time.Duration
	CorrelationCooldownTTL             *time.Duration
	InvestigationTimeout               *time.Duration
	AgentPresenceTTL                   *time.Duration
	AgentDisconnectGrace               *time.Duration
	SchedulerLeaderTTL                 *time.Duration
	SlackIncidentChannelsEnabled       *bool
	SlackIncidentChannelVisibility     *string
	SlackIncidentChannelTriggerStatus  *string
	SlackIncidentChannelArchiveOnClose *bool
	IncidentSummaryEnabled             *bool
	IncidentSummaryInterval            *time.Duration
	// IncidentSummaryIntervals is nil when the field was not provided; a
	// non-nil (possibly empty) map replaces all per-severity overrides.
	IncidentSummaryIntervals map[string]time.Duration

	// Authentication.
	GoogleOAuthEnabled     *bool
	GoogleClientID         *string
	GoogleClientSecret     *string // plaintext; only set when the caller provides a new value
	GoogleOAuthRedirectURL *string
}

func (u *systemConfigUpdate) anySet() bool {
	return u.LogLevel != nil || u.SessionExpiryHrs != nil || u.MaxConcurrentInvestigations != nil ||
		u.CorrelationWindow != nil || u.CorrelationCooldownTTL != nil || u.InvestigationTimeout != nil ||
		u.AgentPresenceTTL != nil || u.AgentDisconnectGrace != nil || u.SchedulerLeaderTTL != nil ||
		u.SlackIncidentChannelsEnabled != nil || u.SlackIncidentChannelVisibility != nil ||
		u.SlackIncidentChannelTriggerStatus != nil || u.SlackIncidentChannelArchiveOnClose != nil ||
		u.IncidentSummaryEnabled != nil || u.IncidentSummaryInterval != nil || u.IncidentSummaryIntervals != nil ||
		u.GoogleOAuthEnabled != nil || u.GoogleClientID != nil || u.GoogleClientSecret != nil ||
		u.GoogleOAuthRedirectURL != nil
}

// parseOptionalDuration validates an optional Go-duration string field. Empty
// input means "not provided".
func parseOptionalDuration(raw, field string) (*time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		if strings.TrimSpace(raw) == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("invalid %s duration: %s", field, err.Error())
	}
	return &d, nil
}

// parseSystemConfigUpdate decodes and validates the PUT /system/config body.
// It returns a user-facing validation error for malformed fields.
func parseSystemConfigUpdate(req map[string]any) (*systemConfigUpdate, error) {
	upd := &systemConfigUpdate{}

	if v, ok := req["log_level"].(string); ok {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			switch v {
			case "debug", "info", "warn", "error", "fatal":
				upd.LogLevel = &v
			default:
				return nil, errors.New("log_level must be one of: debug, info, warn, error, fatal")
			}
		}
	}

	if v, ok := req["session_expiry_hours"].(float64); ok {
		n := int(v)
		if n > 0 && n <= 720 {
			upd.SessionExpiryHrs = &n
		}
	}

	if v, ok := req["max_concurrent_investigations"].(float64); ok {
		n := int(v)
		if n > 0 {
			upd.MaxConcurrentInvestigations = &n
		}
	}

	for _, f := range []struct {
		key   string
		dest  **time.Duration
		blank string
	}{
		{"correlation_window", &upd.CorrelationWindow, ""},
		{"correlation_cooldown_ttl", &upd.CorrelationCooldownTTL, ""},
		{"investigation_timeout", &upd.InvestigationTimeout, ""},
		{"agent_presence_ttl", &upd.AgentPresenceTTL, ""},
		{"agent_disconnect_grace", &upd.AgentDisconnectGrace, ""},
		{"scheduler_leader_ttl", &upd.SchedulerLeaderTTL, ""},
	} {
		if v, ok := req[f.key].(string); ok {
			d, err := parseOptionalDuration(v, f.key)
			if err != nil {
				return nil, err
			}
			*f.dest = d
		}
	}

	if v, ok := req["slack_incident_channels_enabled"].(bool); ok {
		upd.SlackIncidentChannelsEnabled = &v
	}
	if v, ok := req["slack_incident_channel_visibility"].(string); ok {
		if v != "public" && v != "private" {
			return nil, errors.New("slack_incident_channel_visibility must be 'public' or 'private'")
		}
		upd.SlackIncidentChannelVisibility = &v
	}
	if v, ok := req["slack_incident_channel_trigger_status"].(string); ok {
		if v != "active" && v != "detected" {
			return nil, errors.New("slack_incident_channel_trigger_status must be 'active' or 'detected'")
		}
		upd.SlackIncidentChannelTriggerStatus = &v
	}
	if v, ok := req["slack_incident_channel_archive_on_close"].(bool); ok {
		upd.SlackIncidentChannelArchiveOnClose = &v
	}

	if v, ok := req["incident_summary_enabled"].(bool); ok {
		upd.IncidentSummaryEnabled = &v
	}
	if v, ok := req["incident_summary_interval"].(string); ok {
		d, err := parseOptionalDuration(v, "incident_summary_interval")
		if err != nil {
			return nil, err
		}
		upd.IncidentSummaryInterval = d
	}
	if v, ok := req["incident_summary_intervals"].(map[string]any); ok {
		intervals := make(map[string]time.Duration, len(v))
		for sev, val := range v {
			s, ok := val.(string)
			if !ok {
				return nil, errors.New("incident_summary_intervals values must be Go duration strings")
			}
			d, err := time.ParseDuration(strings.TrimSpace(s))
			if err != nil {
				return nil, fmt.Errorf("invalid incident_summary_intervals duration for %s: %s", sev, err.Error())
			}
			intervals[strings.ToLower(strings.TrimSpace(sev))] = d
		}
		upd.IncidentSummaryIntervals = intervals
	}

	// --- Authentication: Google OAuth ---
	if v, ok := req["google_oauth_enabled"].(bool); ok {
		upd.GoogleOAuthEnabled = &v
	}
	if v, ok := req["google_client_id"].(string); ok {
		s := strings.TrimSpace(v)
		upd.GoogleClientID = &s
	}
	if v, ok := req["google_client_secret"].(string); ok {
		// Only accept a non-empty value; an empty string means "leave
		// unchanged" because the secret is never returned on GET.
		if s := strings.TrimSpace(v); s != "" {
			upd.GoogleClientSecret = &s
		}
	}
	if v, ok := req["google_oauth_redirect_url"].(string); ok {
		s := strings.TrimSpace(v)
		upd.GoogleOAuthRedirectURL = &s
	}

	return upd, nil
}

// applySystemConfigUpdate mutates the live config under the server lock.
// summaryChanged reports whether the incident-summary scheduler needs a
// reconfiguration. Callers must compute googleSecret handling before calling.
func (s *Server) applySystemConfigUpdate(upd *systemConfigUpdate) (summaryChanged bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if upd.LogLevel != nil {
		s.cfg.LogLevel = *upd.LogLevel
		logger.Init(*upd.LogLevel, s.cfg.LogFile)
	}
	if upd.SessionExpiryHrs != nil {
		s.cfg.SessionExpiryHrs = *upd.SessionExpiryHrs
	}
	if upd.MaxConcurrentInvestigations != nil {
		s.cfg.MaxConcurrentInvestigations = *upd.MaxConcurrentInvestigations
	}
	if upd.CorrelationWindow != nil {
		s.cfg.CorrelationWindow = *upd.CorrelationWindow
	}
	if upd.CorrelationCooldownTTL != nil {
		s.cfg.CorrelationCooldownTTL = *upd.CorrelationCooldownTTL
	}
	if upd.InvestigationTimeout != nil {
		s.cfg.InvestigationTimeout = *upd.InvestigationTimeout
	}
	if upd.AgentPresenceTTL != nil {
		s.cfg.AgentPresenceTTL = *upd.AgentPresenceTTL
	}
	if upd.AgentDisconnectGrace != nil {
		s.cfg.AgentDisconnectGrace = *upd.AgentDisconnectGrace
	}
	if upd.SchedulerLeaderTTL != nil {
		s.cfg.SchedulerLeaderTTL = *upd.SchedulerLeaderTTL
	}
	if upd.SlackIncidentChannelsEnabled != nil {
		s.cfg.SlackIncidentChannelsEnabled = *upd.SlackIncidentChannelsEnabled
	}
	if upd.SlackIncidentChannelVisibility != nil {
		s.cfg.SlackIncidentChannelVisibility = *upd.SlackIncidentChannelVisibility
	}
	if upd.SlackIncidentChannelTriggerStatus != nil {
		s.cfg.SlackIncidentChannelTriggerStatus = *upd.SlackIncidentChannelTriggerStatus
	}
	if upd.SlackIncidentChannelArchiveOnClose != nil {
		s.cfg.SlackIncidentChannelArchiveOnClose = *upd.SlackIncidentChannelArchiveOnClose
	}

	if upd.IncidentSummaryEnabled != nil {
		s.cfg.IncidentSummaryEnabled = *upd.IncidentSummaryEnabled
		summaryChanged = true
	}
	if upd.IncidentSummaryInterval != nil {
		s.cfg.IncidentSummaryInterval = *upd.IncidentSummaryInterval
		summaryChanged = true
	}
	if upd.IncidentSummaryIntervals != nil {
		s.cfg.IncidentSummaryIntervals = upd.IncidentSummaryIntervals
		summaryChanged = true
	}

	// --- Authentication ---
	if upd.GoogleOAuthEnabled != nil {
		s.cfg.GoogleOAuthEnabled = *upd.GoogleOAuthEnabled
	}
	if upd.GoogleClientID != nil {
		s.cfg.GoogleClientID = *upd.GoogleClientID
	}
	if upd.GoogleClientSecret != nil {
		s.cfg.GoogleClientSecret = *upd.GoogleClientSecret
	}
	if upd.GoogleOAuthRedirectURL != nil {
		s.cfg.GoogleOAuthRedirectURL = *upd.GoogleOAuthRedirectURL
	}

	return summaryChanged
}

func (s *Server) handlePutSystemConfig(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.SystemConfigWrite) {
		return
	}

	var req map[string]any
	if !decodeJSON(w, r, &req) {
		return
	}

	upd, err := parseSystemConfigUpdate(req)
	if err != nil {
		writeError(w, ErrorCodeValidationFailed, err.Error())
		return
	}

	if !upd.anySet() {
		writeStatus(w, "no changes")
		return
	}

	// Encrypt the secret before mutating anything so an encryption failure
	// aborts the update and preserves the stored ciphertext. When no new
	// secret is provided, re-encrypt the existing in-memory secret so an
	// unrelated update does not overwrite the stored credential with empty.
	secretToEncrypt := ""
	if upd.GoogleClientSecret != nil {
		secretToEncrypt = *upd.GoogleClientSecret
	} else {
		s.mu.RLock()
		secretToEncrypt = s.cfg.GoogleClientSecret
		s.mu.RUnlock()
	}
	googleSecretEnc, err := encryptAuthSecret(secretToEncrypt)
	if err != nil {
		logger.ErrorCtx(r.Context(), "failed to encrypt google client secret; aborting system config update", "error", err)
		writeErrorStatus(w, http.StatusInternalServerError, ErrorCodeInternal, "failed to encrypt google client secret")
		return
	}

	summaryChanged := s.applySystemConfigUpdate(upd)

	if summaryChanged && s.summaryConfigApplier != nil {
		s.mu.RLock()
		interval := s.cfg.IncidentSummaryInterval
		if interval <= 0 {
			interval = 15 * time.Minute
		}
		enabled := s.cfg.IncidentSummaryEnabled
		intervals := s.cfg.IncidentSummaryIntervals
		s.mu.RUnlock()
		s.summaryConfigApplier(enabled, interval, intervals)
	}

	if s.systemConfigStore != nil {
		dbCfg := store.SystemConfigValues{
			CorrelationWindow:                  durationStr(s.cfg.CorrelationWindow),
			CorrelationCooldownTTL:             durationStr(s.cfg.CorrelationCooldownTTL),
			InvestigationTimeout:               durationStr(s.cfg.InvestigationTimeout),
			MaxConcurrentInvestigations:        s.cfg.MaxConcurrentInvestigations,
			AgentPresenceTTL:                   durationStr(s.cfg.AgentPresenceTTL),
			AgentDisconnectGrace:               durationStr(s.cfg.AgentDisconnectGrace),
			SchedulerLeaderTTL:                 durationStr(s.cfg.SchedulerLeaderTTL),
			SessionExpiryHours:                 s.cfg.SessionExpiryHrs,
			LogLevel:                           s.cfg.LogLevel,
			SlackIncidentChannelsEnabled:       s.cfg.SlackIncidentChannelsEnabled,
			SlackIncidentChannelVisibility:     s.cfg.SlackIncidentChannelVisibility,
			SlackIncidentChannelTriggerStatus:  s.cfg.SlackIncidentChannelTriggerStatus,
			SlackIncidentChannelArchiveOnClose: s.cfg.SlackIncidentChannelArchiveOnClose,
			IncidentSummaryEnabled:             s.cfg.IncidentSummaryEnabled,
			IncidentSummaryInterval:            durationStr(s.cfg.IncidentSummaryInterval),
			IncidentSummaryIntervals:           durationIntervalsToAny(s.cfg.IncidentSummaryIntervals),

			GoogleOAuthEnabled:     s.cfg.GoogleOAuthEnabled,
			GoogleClientID:         s.cfg.GoogleClientID,
			GoogleClientSecretEnc:  googleSecretEnc,
			GoogleOAuthRedirectURL: s.cfg.GoogleOAuthRedirectURL,
		}
		if err := s.systemConfigStore.Save(dbCfg); err != nil {
			logger.ErrorCtx(r.Context(), "failed to persist system config to DB", "error", err)
		}
	}

	s.systemConfigUpdatedAt = time.Now().UTC()

	if user := userFromContext(r.Context()); user != nil {
		auditFields := make(map[string]any, len(req))
		for k, v := range req {
			auditFields[k] = v
		}
		if _, ok := auditFields["google_client_secret"]; ok {
			auditFields["google_client_secret"] = "[redacted]"
		}
		s.audit(r, store.AuditSystemConfigUpdated, map[string]any{
			"fields": auditFields,
		})
	}

	writeStatus(w, "updated")
}

func durationStr(d time.Duration) string {
	if d == 0 {
		return "0"
	}
	return d.String()
}

// severityIntervalsToStrings renders the per-severity cadence overrides as
// duration strings for the API response. It always returns a non-nil map so
// the JSON serializes to {} instead of null.
func severityIntervalsToStrings(m map[string]time.Duration) map[string]string {
	out := make(map[string]string, len(m))
	for sev, d := range m {
		out[sev] = durationStr(d)
	}
	return out
}

// durationIntervalsToAny renders the per-severity cadence overrides as the
// map[string]any (with string values) shape persisted by the store.
func durationIntervalsToAny(m map[string]time.Duration) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for sev, d := range m {
		out[sev] = durationStr(d)
	}
	return out
}

// encryptAuthSecret encrypts a plaintext auth secret (Google client
// secret) for persistence in the system config. It returns an error when a
// non-empty secret cannot be encrypted so callers can abort instead of
// overwriting the stored ciphertext with an empty value.
func encryptAuthSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	enc, err := algacrypto.Default().EncryptString(plaintext)
	if err != nil {
		return "", fmt.Errorf("encrypt auth secret: %w", err)
	}
	return enc, nil
}
