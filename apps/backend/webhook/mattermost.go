package webhook

import (
	"net/http"
	"strings"
	"time"

	"alga/api/platform"
	algacrypto "alga/crypto"
	"alga/logger"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

// InvestigationAgentForwarder delivers investigation thread messages to the assigned SRE agent.
type InvestigationAgentForwarder interface {
	ForwardToAgent(agentID, investigationID, senderID, senderName, message string) error
	ForwardEventToAgent(agentID string, event sse.Event) error
	AgentOnline(agentID string) bool
}

type MattermostWebhookHandler struct {
	alertInvestigationStore store.AlertInvestigationStore
	auditStore              store.AuditStore
	webhookSecret           string
	investigationForwarder  InvestigationAgentForwarder
	sse                     SSEPublisherMixin
	botUsernames            map[string]bool
	rateLimiter             RateLimiter
}

func NewMattermostWebhookHandler(alertInvestigationStore store.AlertInvestigationStore, auditStore store.AuditStore, webhookSecret string) *MattermostWebhookHandler {
	return &MattermostWebhookHandler{
		alertInvestigationStore: alertInvestigationStore,
		auditStore:              auditStore,
		webhookSecret:           webhookSecret,
		botUsernames:            map[string]bool{"alga": true},
	}
}

func (h *MattermostWebhookHandler) AddBotUsername(name string) {
	if name != "" {
		h.botUsernames[name] = true
	}
}

func (h *MattermostWebhookHandler) SetInvestigationForwarder(forwarder InvestigationAgentForwarder) {
	h.investigationForwarder = forwarder
}

// SetRateLimiter guards Mattermost webhook ingress against volumetric abuse
// of the pre-auth HMAC verification performed on every request.
func (h *MattermostWebhookHandler) SetRateLimiter(rl RateLimiter) {
	h.rateLimiter = rl
}

func (h *MattermostWebhookHandler) SetSSEBroker(broker *sse.Broker, vkClient *valkey.Client) {
	h.sse.SetSSEBroker(broker, vkClient)
}

type MattermostPostEvent struct {
	Token     string `json:"token"`
	PostID    string `json:"post_id"`
	RootID    string `json:"root_id"`
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"user_name"`
	Message   string `json:"message"`
	TeamID    string `json:"team_id"`
	Timestamp int64  `json:"timestamp"`
	EventType string `json:"event_type"`
}

func (h *MattermostWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.rateLimiter != nil && !h.rateLimiter.Allow(clientIPFromRequest(r)) {
		platform.WriteRateLimitExceeded(w, "60")
		return
	}

	if h.webhookSecret == "" {
		logger.Warn("Mattermost webhook accepted without authentication: webhook secret not configured", "component", "webhook")
		http.Error(w, "webhook secret not configured", http.StatusServiceUnavailable)
		return
	}

	bearerToken := ""
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		bearerToken = strings.TrimPrefix(authHeader, "Bearer ")
	}

	if algacrypto.ConstantTimeEqualString(bearerToken, h.webhookSecret) {
		var event MattermostPostEvent
		if !platform.DecodeJSON(w, r, &event) {
			return
		}
		h.handleEvent(w, r, event)
		return
	}

	var event MattermostPostEvent
	if !platform.DecodeJSON(w, r, &event) {
		return
	}
	if !algacrypto.ConstantTimeEqualString(event.Token, h.webhookSecret) {
		logger.Warn("Mattermost webhook rejected: invalid token", "component", "webhook")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.handleEvent(w, r, event)
}

func (h *MattermostWebhookHandler) handleEvent(w http.ResponseWriter, r *http.Request, event MattermostPostEvent) {
	if event.RootID == "" {
		logger.Debug("Skipping Mattermost webhook: not a thread reply", "component", "webhook")
		platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "not a thread reply"})
		return
	}

	// Look up investigation by primary thread ID (the first alert's thread)
	record, err := h.alertInvestigationStore.GetAlertInvestigationByMMThread(r.Context(), event.RootID)
	if err != nil {
		logger.Error("Failed to query investigation by thread ID", "component", "webhook", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if record == nil {
		logger.Debug("No investigation found for Mattermost thread", "component", "webhook", "root_id", event.RootID)
		platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "no matching investigation"})
		return
	}

	if alertInvestigationHasUpdateMMPostID(record, event.PostID) {
		logger.Debug("Skipping Mattermost webhook: post already recorded for investigation", "component", "webhook", "post_id", event.PostID, "investigation_id", record.AlertInvestigationID)
		platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "duplicate_post"})
		return
	}

	update := store.InvestigationUpdate{
		Type:      store.UpdateTypeComment,
		Message:   event.Message,
		Source:    store.UpdateSourceMattermost,
		CreatedAt: time.Now(),
	}
	if isInternal, cleaned := parseInternalNote(event.Message); isInternal {
		update.Internal = true
		update.Message = cleaned
	}
	if event.Username != "" {
		update.Username = &event.Username
	}
	if event.UserID != "" {
		update.UserID = &event.UserID
	}
	update.MMPostID = event.PostID

	if err := h.alertInvestigationStore.AddAlertInvestigationUpdate(r.Context(), record.AlertInvestigationID, update); err != nil {
		logger.Error("Failed to add investigation update from Mattermost", "component", "webhook", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	actorName := event.Username
	if actorName == "" {
		actorName = event.UserID
	}
	h.auditStore.Log(store.AuditInvestigationUpdated, nil, "mattermost:"+actorName, "", "", true, map[string]any{
		"investigation_id": record.AlertInvestigationID,
		"update_type":      "comment",
		"source":           "mattermost",
		"mm_post_id":       event.PostID,
	})

	h.sse.PublishInvestigationEvent("investigation_update", map[string]any{
		"alert_investigation_id": record.AlertInvestigationID,
		"update":                 update,
		"status":                 record.Status,
	})

	logger.Info("Added investigation update from Mattermost", "component", "webhook", "investigation_id", record.AlertInvestigationID)

	if h.botUsernames[event.Username] {
		logger.Debug("Skipping Hermes forward: message from bot user", "component", "webhook", "username", event.Username)
	} else if h.investigationForwarder != nil && record.AgentID != "" && ShouldForwardToAgent(event.Message) {
		if err := h.investigationForwarder.ForwardToAgent(record.AgentID, record.AlertInvestigationID, event.UserID, event.Username, event.Message); err != nil {
			logger.Error("Failed to forward message to agent", "component", "webhook", "error", err)
		} else {
			logger.Debug("Forwarded Mattermost reply to agent", "component", "webhook", "investigation_id", record.AlertInvestigationID)
		}
	}

	platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "investigation_id": record.AlertInvestigationID})
}

// ShouldForwardToAgent returns false for internal-only messages (e.g. leading lock emoji).
func ShouldForwardToAgent(message string) bool {
	trimmed := strings.TrimSpace(message)
	return !strings.HasPrefix(trimmed, "🔒")
}
