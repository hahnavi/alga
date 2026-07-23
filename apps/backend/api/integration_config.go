// Code moved from http.go; see git history.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"alga/config"
	"alga/logger"
	"alga/mattermost"
	"alga/rbac"
	"alga/routing"
	"alga/slack"
	"alga/store"
	"alga/valkey"
)

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.routeRulesStore == nil {
			writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "route rules store not available")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		routesJSON, err := s.cache.GetOrSet(ctx, valkey.PrefixRoutes, valkey.TTLRoutes, func(ctx context.Context) ([]byte, error) {
			routes, err := s.routeRulesStore.Get()
			if err != nil {
				return nil, err
			}
			defaultDests := s.getDefaultDestinations()
			resp := map[string]any{
				"routes": routes,
			}
			if len(defaultDests) > 0 {
				resp["default_destinations"] = defaultDests
			}
			return json.Marshal(resp)
		})
		if err != nil {
			writeInternalError(w, err, "failed to get routes")
			return
		}

		writeRawJSON(w, http.StatusOK, routesJSON)
	case http.MethodPut:
		if !s.checkPermission(w, r, rbac.RoutesWrite) {
			return
		}
		if s.routeRulesStore == nil {
			writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "route rules store not available")
			return
		}
		var req struct {
			Routes []config.RouteConfig `json:"routes"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if err := validateRouteConfigs(req.Routes); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
			return
		}

		if err := s.routeRulesStore.Save(req.Routes); err != nil {
			writeInternalError(w, err, "failed to save routes")
			return
		}

		if s.onRoutesChanged != nil {
			engine := routing.NewEngine(req.Routes)
			engine.SetDefaults(s.getDefaultRoutingDestinations())
			s.onRoutesChanged(engine)
		}

		s.audit(r, store.AuditRoutesUpdated, map[string]any{
			"routes_count": len(req.Routes),
		})

		if s.cache != nil {
			_ = s.cache.Invalidate(r.Context(), valkey.PrefixRoutes)
		}

		writeStatus(w, "updated")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	p := s.chatRouter.Provider("mattermost")
	if p == nil || !p.Enabled() {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "mattermost integration not configured")
		return
	}
	channels, err := p.ListChannels(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to list channels")
		return
	}
	writeData(w, http.StatusOK, channels)
}

func (s *Server) handleDestinations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	provider := r.URL.Query().Get("provider")
	p := s.chatRouter.Provider(provider)
	resolvedProvider := provider
	if provider == "" {
		p = s.chatRouter.Provider("mattermost")
		resolvedProvider = "mattermost"
	}
	if p == nil || !p.Enabled() {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, resolvedProvider+" integration not configured")
		return
	}
	channels, err := p.ListChannels(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to list channels")
		return
	}
	writeData(w, http.StatusOK, channels)
}

func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetIntegrations(w, r)
	case http.MethodPut:
		s.handlePutIntegrations(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) buildDefaultRoutingDestinations() []routing.Destination {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var defaults []routing.Destination
	if s.cfg.MattermostDefaultChannel != "" && s.mmClient != nil && s.mmClient.Enabled() {
		defaults = append(defaults, routing.Destination{Provider: "mattermost", Channel: s.cfg.MattermostDefaultChannel})
	}
	if s.cfg.SlackDefaultChannel != "" {
		defaults = append(defaults, routing.Destination{Provider: "slack", Channel: s.cfg.SlackDefaultChannel})
	}
	return defaults
}

func (s *Server) getDefaultDestinations() []config.RouteTarget {
	routingDests := s.buildDefaultRoutingDestinations()
	targets := make([]config.RouteTarget, len(routingDests))
	for i, d := range routingDests {
		targets[i] = config.RouteTarget{Provider: d.Provider, Channel: d.Channel}
	}
	return targets
}

func (s *Server) getDefaultRoutingDestinations() []routing.Destination {
	return s.buildDefaultRoutingDestinations()
}

func (s *Server) rebuildRoutingDefaults() {
	if s.onRoutesChanged == nil {
		return
	}
	defaults := s.buildDefaultRoutingDestinations()

	routes, err := s.routeRulesStore.Get()
	if err != nil {
		logger.Error("failed to load routes for default rebuild", "error", err)
		return
	}
	engine := routing.NewEngine(routes)
	engine.SetDefaults(defaults)
	s.onRoutesChanged(engine)
}

func (s *Server) loadIntegrationState() (mmURL, mmSecret, mmTeam, slackToken, slackSigningSecret string) {
	s.mu.RLock()
	mmURL = s.cfg.MattermostURL
	mmSecret = s.cfg.MattermostWebhookSecret
	mmTeam = s.cfg.MattermostTeam
	slackToken = s.cfg.SlackBotToken
	slackSigningSecret = s.cfg.SlackSigningSecret
	s.mu.RUnlock()

	if s.integrationStore != nil {
		stored, err := s.integrationStore.Get()
		if err != nil {
			logger.Error("Failed to load stored integrations", "error", err)
		} else if stored != nil {
			if mmURL == "" {
				mmURL = stored.MattermostURL
			}
			if mmSecret == "" {
				mmSecret = stored.MattermostWebhookSecret
			}
			if mmTeam == "" {
				mmTeam = stored.MattermostTeam
			}
			if slackToken == "" {
				slackToken = stored.SlackBotToken
			}
			if slackSigningSecret == "" {
				slackSigningSecret = stored.SlackSigningSecret
			}
		}
	}
	return
}

func (s *Server) handleGetIntegrations(w http.ResponseWriter, r *http.Request) {
	if s.integrationStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "integration store not available")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	integJSON, err := s.cache.GetOrSet(ctx, valkey.PrefixIntegrations, valkey.TTLIntegrations, func(ctx context.Context) ([]byte, error) {
		mmURL, mmSecret, mmTeam, slackToken, slackSigningSecret := s.loadIntegrationState()

		mmURLLocked := os.Getenv("MATTERMOST_SERVER_URL") != ""
		slackLocked := os.Getenv("SLACK_BOT_TOKEN") != "" || os.Getenv("SLACK_DEFAULT_CHANNEL") != "" || os.Getenv("SLACK_DISABLED") != "" || os.Getenv("SLACK_SIGNING_SECRET") != "" || os.Getenv("SLACK_CLIENT_ID") != "" || os.Getenv("SLACK_CLIENT_SECRET") != ""
		twilioLocked := os.Getenv("TWILIO_ACCOUNT_SID") != ""
		telnyxLocked := os.Getenv("TELNYX_API_KEY") != ""
		voiceProviderLocked := os.Getenv("VOICE_PROVIDER") != ""
		activeProvider := config.NormalizeVoiceProvider(s.cfg.VoiceProvider)

		mmEnabled := mmURL != "" && mmSecret != ""
		slackEnabled := slackToken != ""

		var mmDefaultChannel, slackDefaultChannel string
		var slackClientID, slackWorkspaceName, slackWorkspaceID string
		var twilioSID, twilioToken, twilioFrom string
		var twilioDisabled bool
		var telnyxAPIKey, telnyxConnID, telnyxFrom, telnyxPublicKey string
		var telnyxTTSVoice, telnyxTTSLanguage, telnyxTTSAPIKeyRef string
		var telnyxDisabled bool
		if s.integrationStore != nil {
			stored, err := s.integrationStore.Get()
			if err == nil && stored != nil {
				mmDefaultChannel = stored.MattermostDefaultChannel
				slackDefaultChannel = stored.SlackDefaultChannel
				slackClientID = stored.SlackClientID
				slackWorkspaceName = stored.SlackWorkspaceName
				slackWorkspaceID = stored.SlackWorkspaceID
				twilioSID = stored.TwilioAccountSID
				twilioToken = stored.TwilioAuthToken
				twilioFrom = stored.TwilioFromNumber
				twilioDisabled = stored.TwilioDisabled
				telnyxAPIKey = stored.TelnyxAPIKey
				telnyxConnID = stored.TelnyxConnectionID
				telnyxFrom = stored.TelnyxFromNumber
				telnyxPublicKey = stored.TelnyxPublicKey
				telnyxTTSVoice = stored.TelnyxTTSVoice
				telnyxTTSLanguage = stored.TelnyxTTSLanguage
				telnyxTTSAPIKeyRef = stored.TelnyxTTSAPIKeyRef
				telnyxDisabled = stored.TelnyxDisabled
			}
		}
		s.mu.RLock()
		mmDisabled := s.cfg.MattermostDisabled
		slackDisabled := s.cfg.SlackDisabled
		googleMeetEnabled := s.cfg.GoogleMeetEnabled && s.cfg.GoogleMeetCredentialsPath != ""
		googleMeetAutoCreate := s.cfg.GoogleMeetAutoCreate
		if s.cfg.MattermostDefaultChannel != "" {
			mmDefaultChannel = s.cfg.MattermostDefaultChannel
		}
		if s.cfg.SlackDefaultChannel != "" {
			slackDefaultChannel = s.cfg.SlackDefaultChannel
		}
		if s.cfg.SlackClientID != "" {
			slackClientID = s.cfg.SlackClientID
		}
		if s.cfg.SlackWorkspaceName != "" {
			slackWorkspaceName = s.cfg.SlackWorkspaceName
		}
		if s.cfg.SlackWorkspaceID != "" {
			slackWorkspaceID = s.cfg.SlackWorkspaceID
		}
		if s.cfg.TwilioAccountSID != "" {
			twilioSID = s.cfg.TwilioAccountSID
		}
		if s.cfg.TwilioAuthToken != "" {
			twilioToken = s.cfg.TwilioAuthToken
		}
		if s.cfg.TwilioFromNumber != "" {
			twilioFrom = s.cfg.TwilioFromNumber
		}
		if os.Getenv("TWILIO_DISABLED") != "" {
			twilioDisabled = s.cfg.TwilioDisabled
		}
		if s.cfg.TelnyxAPIKey != "" {
			telnyxAPIKey = s.cfg.TelnyxAPIKey
		}
		if s.cfg.TelnyxConnectionID != "" {
			telnyxConnID = s.cfg.TelnyxConnectionID
		}
		if s.cfg.TelnyxFromNumber != "" {
			telnyxFrom = s.cfg.TelnyxFromNumber
		}
		if s.cfg.TelnyxPublicKey != "" {
			telnyxPublicKey = s.cfg.TelnyxPublicKey
		}
		if s.cfg.TelnyxTTSVoice != "" {
			telnyxTTSVoice = s.cfg.TelnyxTTSVoice
		}
		if s.cfg.TelnyxTTSLanguage != "" {
			telnyxTTSLanguage = s.cfg.TelnyxTTSLanguage
		}
		if s.cfg.TelnyxTTSAPIKeyRef != "" {
			telnyxTTSAPIKeyRef = s.cfg.TelnyxTTSAPIKeyRef
		}
		if os.Getenv("TELNYX_DISABLED") != "" {
			telnyxDisabled = s.cfg.TelnyxDisabled
		}
		s.mu.RUnlock()

		return json.Marshal(map[string]any{
			"mattermost": map[string]any{
				"enabled":           mmEnabled,
				"provider_enabled":  !mmDisabled,
				"url":               maskURL(mmURL),
				"base_url":          strings.TrimSuffix(mmURL, "/"),
				"secret_configured": mmSecret != "",
				"team":              mmTeam,
				"locked":            mmURLLocked,
				"default_channel":   mmDefaultChannel,
			},
			"slack": map[string]any{
				"enabled":                   slackEnabled,
				"provider_enabled":          !slackDisabled,
				"token_configured":          slackToken != "",
				"signing_secret_configured": slackSigningSecret != "",
				"client_id_configured":      slackClientID != "",
				"workspace_name":            slackWorkspaceName,
				"workspace_id":              slackWorkspaceID,
				"locked":                    slackLocked,
				"default_channel":           slackDefaultChannel,
			},
			"twilio": map[string]any{
				"enabled":                twilioSID != "" && twilioToken != "" && twilioFrom != "",
				"provider_enabled":       !twilioDisabled,
				"active":                 activeProvider == "twilio",
				"account_sid_configured": twilioSID != "",
				"auth_token_configured":  twilioToken != "",
				"from_number":            twilioFrom,
				"locked":                 twilioLocked,
			},
			"telnyx": map[string]any{
				"enabled":               telnyxAPIKey != "" && telnyxConnID != "" && telnyxFrom != "",
				"provider_enabled":      !telnyxDisabled,
				"active":                activeProvider == "telnyx",
				"api_key_configured":    telnyxAPIKey != "",
				"connection_id":         telnyxConnID,
				"from_number":           telnyxFrom,
				"public_key_configured": telnyxPublicKey != "",
				"tts_voice":             telnyxTTSVoice,
				"tts_language":          telnyxTTSLanguage,
				"tts_api_key_ref":       telnyxTTSAPIKeyRef,
				"locked":                telnyxLocked,
			},
			"voice_provider":        activeProvider,
			"voice_provider_locked": voiceProviderLocked,
			"google_meet": map[string]any{
				"enabled":     googleMeetEnabled,
				"auto_create": googleMeetEnabled && googleMeetAutoCreate,
			},
		})
	})
	if err != nil {
		writeInternalError(w, err, "failed to get integrations")
		return
	}

	writeRawJSON(w, http.StatusOK, integJSON)
}

func (s *Server) handlePutIntegrations(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.IntegrationsWrite) {
		return
	}

	var req struct {
		VoiceProvider string `json:"voice_provider"`
		Mattermost    struct {
			URL             string `json:"url"`
			Secret          string `json:"secret"`
			Team            string `json:"team"`
			DefaultChannel  string `json:"default_channel"`
			ProviderEnabled *bool  `json:"provider_enabled"`
		} `json:"mattermost"`
		Slack struct {
			BotToken        string `json:"bot_token"`
			SigningSecret   string `json:"signing_secret"`
			DefaultChannel  string `json:"default_channel"`
			ProviderEnabled *bool  `json:"provider_enabled"`
			ClientID        string `json:"client_id"`
			ClientSecret    string `json:"client_secret"`
		} `json:"slack"`
		Twilio struct {
			AccountSID      string `json:"account_sid"`
			AuthToken       string `json:"auth_token"`
			FromNumber      string `json:"from_number"`
			ProviderEnabled *bool  `json:"provider_enabled"`
		} `json:"twilio"`
		Telnyx struct {
			APIKey          string `json:"api_key"`
			ConnectionID    string `json:"connection_id"`
			FromNumber      string `json:"from_number"`
			PublicKey       string `json:"public_key"`
			TTSVoice        string `json:"tts_voice"`
			TTSLanguage     string `json:"tts_language"`
			TTSAPIKeyRef    string `json:"tts_api_key_ref"`
			ProviderEnabled *bool  `json:"provider_enabled"`
		} `json:"telnyx"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	mmURLEnv := os.Getenv("MATTERMOST_SERVER_URL")
	slackTokenEnv := os.Getenv("SLACK_BOT_TOKEN")
	slackChannelEnv := os.Getenv("SLACK_DEFAULT_CHANNEL")
	slackDisabledEnv := os.Getenv("SLACK_DISABLED")

	var blocked []string
	if mmURLEnv != "" {
		blocked = append(blocked, "MATTERMOST_SERVER_URL")
	}
	if slackTokenEnv != "" {
		blocked = append(blocked, "SLACK_BOT_TOKEN")
	}
	if slackChannelEnv != "" {
		blocked = append(blocked, "SLACK_DEFAULT_CHANNEL")
	}
	if slackDisabledEnv != "" {
		blocked = append(blocked, "SLACK_DISABLED")
	}
	if os.Getenv("TELNYX_API_KEY") != "" {
		blocked = append(blocked, "TELNYX_API_KEY")
	}
	if len(blocked) > 0 {
		writeError(w, ErrorCodeConflict, fmt.Sprintf("Cannot update integrations: the following are managed by environment variables: %s. Remove the env vars to edit via UI.", strings.Join(blocked, ", ")))
		return
	}

	mmURL := req.Mattermost.URL
	mmTeam := req.Mattermost.Team
	mmDefaultChannel := strings.TrimSpace(req.Mattermost.DefaultChannel)
	slackToken := req.Slack.BotToken
	slackDefaultChannel := strings.TrimSpace(req.Slack.DefaultChannel)

	if s.integrationStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "integration store not available")
		return
	}

	existing, _ := s.integrationStore.Get()
	if existing == nil {
		existing = &store.IntegrationConfig{}
	}

	// Resolve the effective voice provider for this update. Env VOICE_PROVIDER
	// wins (and locks the field); otherwise the request value, then the stored
	// value, then the default "twilio".
	voiceProviderLocked := os.Getenv("VOICE_PROVIDER") != ""
	if voiceProviderLocked && req.VoiceProvider != "" && !strings.EqualFold(req.VoiceProvider, s.cfg.VoiceProvider) {
		writeError(w, ErrorCodeConflict, fmt.Sprintf("Cannot update voice_provider: it is managed by the VOICE_PROVIDER environment variable (set to %q). Remove the env var to edit via UI.", s.cfg.VoiceProvider))
		return
	}
	requestedProvider := config.NormalizeVoiceProvider(req.VoiceProvider)
	if req.VoiceProvider == "" {
		requestedProvider = config.NormalizeVoiceProvider(existing.VoiceProvider)
	}
	if voiceProviderLocked {
		requestedProvider = config.NormalizeVoiceProvider(s.cfg.VoiceProvider)
	}

	if mmURL == "" {
		mmURL = existing.MattermostURL
	}
	if mmTeam == "" {
		mmTeam = existing.MattermostTeam
	}
	if slackToken == "" {
		slackToken = existing.SlackBotToken
	}
	if mmDefaultChannel == "" {
		mmDefaultChannel = existing.MattermostDefaultChannel
	}
	if slackDefaultChannel == "" {
		slackDefaultChannel = existing.SlackDefaultChannel
	}

	slackClientID := req.Slack.ClientID
	if slackClientID == "" {
		slackClientID = existing.SlackClientID
	}
	slackClientSecret := req.Slack.ClientSecret
	if slackClientSecret == "" {
		slackClientSecret = existing.SlackClientSecret
	}

	mmSecret := s.cfg.MattermostWebhookSecret
	if req.Mattermost.Secret != "" {
		mmSecret = req.Mattermost.Secret
	} else if existing.MattermostWebhookSecret != "" {
		mmSecret = existing.MattermostWebhookSecret
	}

	slackSigningSecret := s.cfg.SlackSigningSecret
	if req.Slack.SigningSecret != "" {
		slackSigningSecret = req.Slack.SigningSecret
	} else if existing.SlackSigningSecret != "" {
		slackSigningSecret = existing.SlackSigningSecret
	}

	twilioLocked := os.Getenv("TWILIO_ACCOUNT_SID") != ""
	twilioSID := req.Twilio.AccountSID
	twilioFrom := req.Twilio.FromNumber
	twilioToken := existing.TwilioAuthToken
	if req.Twilio.AuthToken != "" {
		twilioToken = req.Twilio.AuthToken
	}
	if twilioLocked {
		twilioSID = existing.TwilioAccountSID
		twilioFrom = existing.TwilioFromNumber
		twilioToken = existing.TwilioAuthToken
	} else {
		if twilioSID == "" {
			twilioSID = existing.TwilioAccountSID
		}
		if twilioFrom == "" {
			twilioFrom = existing.TwilioFromNumber
		}
	}

	telnyxLocked := os.Getenv("TELNYX_API_KEY") != ""
	telnyxAPIKey := req.Telnyx.APIKey
	telnyxConnID := req.Telnyx.ConnectionID
	telnyxFrom := req.Telnyx.FromNumber
	telnyxPublicKey := req.Telnyx.PublicKey
	telnyxTTSVoice := req.Telnyx.TTSVoice
	telnyxTTSLanguage := req.Telnyx.TTSLanguage
	telnyxTTSAPIKeyRef := req.Telnyx.TTSAPIKeyRef
	if telnyxAPIKey == "" {
		telnyxAPIKey = existing.TelnyxAPIKey
	}
	if telnyxConnID == "" {
		telnyxConnID = existing.TelnyxConnectionID
	}
	if telnyxFrom == "" {
		telnyxFrom = existing.TelnyxFromNumber
	}
	if telnyxPublicKey == "" {
		telnyxPublicKey = existing.TelnyxPublicKey
	}
	if telnyxTTSVoice == "" {
		telnyxTTSVoice = existing.TelnyxTTSVoice
	}
	if telnyxTTSLanguage == "" {
		telnyxTTSLanguage = existing.TelnyxTTSLanguage
	}
	if telnyxTTSAPIKeyRef == "" {
		telnyxTTSAPIKeyRef = existing.TelnyxTTSAPIKeyRef
	}
	if telnyxLocked {
		telnyxAPIKey = existing.TelnyxAPIKey
		telnyxConnID = existing.TelnyxConnectionID
		telnyxFrom = existing.TelnyxFromNumber
		telnyxPublicKey = existing.TelnyxPublicKey
		telnyxTTSVoice = existing.TelnyxTTSVoice
		telnyxTTSLanguage = existing.TelnyxTTSLanguage
		telnyxTTSAPIKeyRef = existing.TelnyxTTSAPIKeyRef
	}

	mmIsConfigured := mmURL != "" && mmSecret != ""
	slackIsConfigured := slackToken != ""

	mmDisabled := existing.MattermostDisabled
	if req.Mattermost.ProviderEnabled != nil {
		mmDisabled = !*req.Mattermost.ProviderEnabled
	}

	slackDisabled := existing.SlackDisabled
	if req.Slack.ProviderEnabled != nil {
		slackDisabled = !*req.Slack.ProviderEnabled
	}

	twilioDisabled := existing.TwilioDisabled
	if req.Twilio.ProviderEnabled != nil {
		twilioDisabled = !*req.Twilio.ProviderEnabled
	}

	telnyxDisabled := existing.TelnyxDisabled
	if req.Telnyx.ProviderEnabled != nil {
		telnyxDisabled = !*req.Telnyx.ProviderEnabled
	}

	// Enforce mutual exclusivity: only the effective provider may be enabled.
	// Reject only when the request explicitly enables the inactive provider;
	// otherwise silently force the inactive provider disabled in the persisted
	// row so the DB can never activate a dormant provider.
	switch requestedProvider {
	case "telnyx":
		if req.Twilio.ProviderEnabled != nil && *req.Twilio.ProviderEnabled {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "Twilio is not the active voice provider; switch voice_provider to \"twilio\" first.")
			return
		}
		twilioDisabled = true
	case "twilio":
		if req.Telnyx.ProviderEnabled != nil && *req.Telnyx.ProviderEnabled {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "Telnyx is not the active voice provider; switch voice_provider to \"telnyx\" first.")
			return
		}
		telnyxDisabled = true
	}

	if !mmDisabled && mmIsConfigured && mmDefaultChannel == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "default_channel is required for mattermost when the integration is configured")
		return
	}
	if !slackDisabled && slackIsConfigured && slackDefaultChannel == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "default_channel is required for slack when the integration is configured")
		return
	}

	merged := store.IntegrationConfig{
		MattermostURL:            mmURL,
		MattermostWebhookSecret:  mmSecret,
		MattermostTeam:           mmTeam,
		MattermostDefaultChannel: mmDefaultChannel,
		MattermostDisabled:       mmDisabled,
		SlackBotToken:            slackToken,
		SlackSigningSecret:       slackSigningSecret,
		SlackDefaultChannel:      slackDefaultChannel,
		SlackDisabled:            slackDisabled,
		SlackClientID:            slackClientID,
		SlackClientSecret:        slackClientSecret,
		SlackWorkspaceName:       existing.SlackWorkspaceName,
		SlackWorkspaceID:         existing.SlackWorkspaceID,
		TwilioAccountSID:         twilioSID,
		TwilioAuthToken:          twilioToken,
		TwilioFromNumber:         twilioFrom,
		TwilioDisabled:           twilioDisabled,
		TelnyxAPIKey:             telnyxAPIKey,
		TelnyxConnectionID:       telnyxConnID,
		TelnyxFromNumber:         telnyxFrom,
		TelnyxPublicKey:          telnyxPublicKey,
		TelnyxTTSVoice:           telnyxTTSVoice,
		TelnyxTTSLanguage:        telnyxTTSLanguage,
		TelnyxTTSAPIKeyRef:       telnyxTTSAPIKeyRef,
		TelnyxDisabled:           telnyxDisabled,
		VoiceProvider:            requestedProvider,
		HermesPlatformURL:        existing.HermesPlatformURL,
		HermesPlatformToken:      existing.HermesPlatformToken,
	}
	if err := s.integrationStore.Save(merged); err != nil {
		writeInternalError(w, err, "failed to save integrations")
		return
	}

	if s.cache != nil {
		_ = s.cache.Invalidate(r.Context(), valkey.PrefixIntegrations)
	}

	s.mu.Lock()
	s.cfg.MattermostURL = mmURL
	if req.Mattermost.Secret != "" {
		s.cfg.MattermostWebhookSecret = req.Mattermost.Secret
	}
	s.cfg.MattermostTeam = mmTeam
	s.cfg.MattermostDefaultChannel = mmDefaultChannel
	s.cfg.MattermostDisabled = mmDisabled
	s.cfg.SlackBotToken = slackToken
	if req.Slack.SigningSecret != "" {
		s.cfg.SlackSigningSecret = req.Slack.SigningSecret
	}
	s.cfg.SlackDefaultChannel = slackDefaultChannel
	s.cfg.TwilioAccountSID = twilioSID
	s.cfg.TwilioAuthToken = twilioToken
	s.cfg.TwilioFromNumber = twilioFrom
	s.cfg.TwilioDisabled = twilioDisabled
	s.cfg.TelnyxAPIKey = telnyxAPIKey
	s.cfg.TelnyxConnectionID = telnyxConnID
	s.cfg.TelnyxFromNumber = telnyxFrom
	s.cfg.TelnyxPublicKey = telnyxPublicKey
	s.cfg.TelnyxTTSVoice = telnyxTTSVoice
	s.cfg.TelnyxTTSLanguage = telnyxTTSLanguage
	s.cfg.TelnyxTTSAPIKeyRef = telnyxTTSAPIKeyRef
	s.cfg.TelnyxDisabled = telnyxDisabled
	if !voiceProviderLocked {
		s.cfg.VoiceProvider = requestedProvider
	}
	s.mu.Unlock()

	mmSecret = s.cfg.MattermostWebhookSecret
	if s.mmClient != nil {
		s.mmClient.Reconfigure(mmURL, mmSecret, mmTeam)
		s.mmClient.SetDisabled(mmDisabled)
	}
	if s.slackClient != nil {
		s.slackClient.Reconfigure(slackToken)
	}
	if s.twilioClient != nil {
		s.twilioClient.Reconfigure(twilioSID, twilioToken, twilioFrom)
		s.twilioClient.SetDisabled(twilioDisabled)
		s.twilioClient.SetCallbackBaseURL(s.cfg.AlgaBaseURL)
	}
	if s.telnyxClient != nil {
		s.telnyxClient.Reconfigure(telnyxAPIKey, telnyxConnID, telnyxFrom, telnyxTTSVoice, telnyxTTSLanguage, telnyxTTSAPIKeyRef)
		if telnyxPublicKey != "" {
			if err := s.telnyxClient.ReconfigurePublicKey(telnyxPublicKey); err != nil {
				logger.Warn("Failed to reconfigure Telnyx public key", "component", "telnyx", "error", err)
			}
		}
		s.telnyxClient.SetDisabled(telnyxDisabled)
		s.telnyxClient.SetCallbackBaseURL(s.cfg.AlgaBaseURL)
	}
	s.rebuildChatRouter()
	if s.slackWebhookHandler != nil {
		s.slackWebhookHandler.SetSigningSecret(s.cfg.SlackSigningSecret)
	}

	s.rebuildRoutingDefaults()

	s.audit(r, store.AuditIntegrationsUpdated, map[string]any{
		"mattermost_enabled": s.mmClient != nil && s.mmClient.Enabled(),
		"slack_enabled":      s.slackClient != nil && s.slackClient.Enabled(),
		"twilio_enabled":     s.twilioClient != nil && s.twilioClient.Enabled(),
		"telnyx_enabled":     s.telnyxClient != nil && s.telnyxClient.Enabled(),
		"voice_provider":     requestedProvider,
	})

	writeStatus(w, "updated")
}

func (s *Server) handleTestIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	var req struct {
		Provider   string `json:"provider"`
		Mattermost struct {
			URL    string `json:"url"`
			Secret string `json:"secret"`
		} `json:"mattermost"`
		Slack struct {
			BotToken string `json:"bot_token"`
		} `json:"slack"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Provider == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "provider is required")
		return
	}

	switch req.Provider {
	case "mattermost":
		if s.mmClient == nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "mattermost not configured")
			return
		}
		mmURL := req.Mattermost.URL
		mmSecret := req.Mattermost.Secret
		if mmURL == "" || mmSecret == "" {
			mmURL, mmSecret, _, _, _ = s.loadIntegrationState()
		}
		if mmURL == "" || mmSecret == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "mattermost not fully configured")
			return
		}
		if isPrivateURL(mmURL) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "URLs pointing to private networks are not allowed")
			return
		}
		testClient := mattermost.NewClient(mmURL, mmSecret, "")
		if err := testClient.TestConnection(r.Context()); err != nil {
			logger.Warn("mattermost connection test failed", "component", "integration-test", "provider", "mattermost", "error", err)
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "mattermost connection test failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Mattermost connection successful"})
	case "slack":
		if s.slackClient == nil {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "slack not configured")
			return
		}
		botToken := req.Slack.BotToken
		if botToken == "" {
			_, _, _, botToken, _ = s.loadIntegrationState()
		}
		if botToken == "" {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "slack bot token not configured")
			return
		}
		testClient := slack.NewClient(botToken)
		if err := testClient.TestConnection(r.Context()); err != nil {
			logger.Warn("slack connection test failed", "component", "integration-test", "provider", "slack", "error", err)
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "slack connection test failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Slack connection successful"})
	default:
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "provider must be one of: mattermost, slack")
	}
}

func maskURL(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••••••"
	}
	return s[:8] + "••••"
}
