package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"alga/api/platform"
	"alga/logger"
	"alga/slack"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

type incidentSlackLookupStore interface {
	GetIncidentBySlackChannel(ctx context.Context, channelID string) (*store.IncidentRecord, error)
}

type SlackWebhookHandler struct {
	alertInvestigationStore   store.AlertInvestigationStore
	auditStore                store.AuditStore
	alertStore                store.Store
	alertBroadcaster          store.AlertEventPublisher
	dedupCache                DedupCache
	slackClient               *slack.Client
	signingSecret             string
	investigationForwarder    InvestigationAgentForwarder
	sse                       SSEPublisherMixin
	botUserIDs                map[string]bool
	incidentLookupStore       incidentSlackLookupStore
	incidentCoordinationStore store.IncidentCoordinationStore
	rateLimiter               RateLimiter
}

func NewSlackWebhookHandler(
	alertInvestigationStore store.AlertInvestigationStore,
	auditStore store.AuditStore,
	signingSecret string,
) *SlackWebhookHandler {
	return &SlackWebhookHandler{
		alertInvestigationStore: alertInvestigationStore,
		auditStore:              auditStore,
		signingSecret:           strings.TrimSpace(signingSecret),
		botUserIDs:              map[string]bool{},
	}
}

func (h *SlackWebhookHandler) SetAlertStore(alertStore store.Store, broadcaster store.AlertEventPublisher, dedupCache DedupCache) {
	h.alertStore = alertStore
	h.alertBroadcaster = broadcaster
	h.dedupCache = dedupCache
}

// SetRateLimiter guards Slack webhook ingress against volumetric abuse of
// the pre-auth signature verification performed on every request.
func (h *SlackWebhookHandler) SetRateLimiter(rl RateLimiter) {
	h.rateLimiter = rl
}

func (h *SlackWebhookHandler) SetSlackClient(client *slack.Client) {
	h.slackClient = client
}

func (h *SlackWebhookHandler) SetInvestigationForwarder(f InvestigationAgentForwarder) {
	h.investigationForwarder = f
}

func (h *SlackWebhookHandler) SetSSEBroker(broker *sse.Broker, vkClient *valkey.Client) {
	h.sse.SetSSEBroker(broker, vkClient)
}

func (h *SlackWebhookHandler) AddBotUserID(id string) {
	if id != "" {
		h.botUserIDs[id] = true
	}
}

func (h *SlackWebhookHandler) handleIncidentCoordinationSlackMessage(ctx context.Context, ev slackMessageEvent) bool {
	if h.incidentLookupStore == nil || h.incidentCoordinationStore == nil || ev.Channel == "" || ev.Text == "" {
		return false
	}
	providerID := store.SlackProviderMessageID(ev.Channel, ev.TS)
	if existing, err := h.incidentCoordinationStore.FindByProviderMessageID(ctx, providerID); err != nil {
		logger.Error("Failed to check incident coordination duplicate", "component", "webhook", "error", err)
		return true
	} else if existing != nil {
		return true
	}
	incident, err := h.incidentLookupStore.GetIncidentBySlackChannel(ctx, ev.Channel)
	if err != nil {
		logger.Error("Failed to query incident by Slack channel", "component", "webhook", "error", err)
		return true
	}
	if incident == nil {
		return false
	}
	threadTS := ev.ThreadTS
	if threadTS == "" {
		threadTS = ev.TS
	}
	record := &store.IncidentCoordinationMessageRecord{
		IncidentNumber:    incident.IncidentNumber,
		Kind:              store.IncidentCoordinationKindChat,
		ActorType:         store.IncidentCoordinationActorExternal,
		ActorDisplayName:  ev.User,
		Body:              ev.Text,
		Source:            store.IncidentCoordinationSourceSlack,
		SlackChannelID:    ev.Channel,
		SlackMessageTS:    ev.TS,
		SlackThreadTS:     threadTS,
		ProviderMessageID: providerID,
		Metadata:          map[string]any{"slack_user_id": ev.User},
	}
	if _, err := h.incidentCoordinationStore.CreateMessage(ctx, record); err != nil {
		logger.Error("Failed to create incident coordination message from Slack", "component", "webhook", "error", err)
		return true
	}
	h.sse.PublishInvestigationEvent("incident_coordination_message_created", map[string]string{"incident_number": strconv.FormatInt(incident.IncidentNumber, 10)})
	return true
}

func (h *SlackWebhookHandler) SetSigningSecret(secret string) {
	h.signingSecret = strings.TrimSpace(secret)
}

func (h *SlackWebhookHandler) SetIncidentLookupStore(st incidentSlackLookupStore) {
	h.incidentLookupStore = st
}

func (h *SlackWebhookHandler) SetIncidentCoordinationStore(st store.IncidentCoordinationStore) {
	h.incidentCoordinationStore = st
}

func (h *SlackWebhookHandler) publishAlertUpdated(record *store.AlertRecord) {
	if h.alertBroadcaster == nil || record == nil {
		return
	}
	h.alertBroadcaster.PublishAlertEvent("alert_updated", *record)
}

type slackURLVerification struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
}

type slackEnvelope struct {
	Type  string          `json:"type"`
	Event json.RawMessage `json:"event"`
}

type slackMessageEvent struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Text     string `json:"text"`
	Channel  string `json:"channel"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	BotID    string `json:"bot_id"`
	Subtype  string `json:"subtype"`
}

type slackInteractionPayload struct {
	Type    string `json:"type"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"user"`
	Channel struct {
		ID string `json:"id"`
	} `json:"channel"`
	Message struct {
		TS     string          `json:"ts"`
		Blocks json.RawMessage `json:"blocks"`
		Text   string          `json:"text"`
	} `json:"message"`
	ResponseURL string `json:"response_url"`
	TriggerID   string `json:"trigger_id"`
}

func (h *SlackWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		platform.WriteErrorStatus(w, http.StatusMethodNotAllowed, platform.ErrorCodeInternal, "method not allowed")
		return
	}
	if h.rateLimiter != nil && !h.rateLimiter.Allow(clientIPFromRequest(r)) {
		platform.WriteRateLimitExceeded(w, "60")
		return
	}
	if h.signingSecret == "" {
		platform.WriteErrorStatus(w, http.StatusServiceUnavailable, platform.ErrorCodeInternal, "slack webhook not configured")
		return
	}

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "bad request")
			return
		}

		ts := r.Header.Get("X-Slack-Request-Timestamp")
		sig := r.Header.Get("X-Slack-Signature")
		if !verifySlackRequest(h.signingSecret, ts, body, sig) {
			logger.Warn("Slack webhook rejected: invalid signature", "component", "webhook")
			webhookError(w, platform.ErrorCodeUnauthorized, "unauthorized")
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		if err := r.ParseForm(); err != nil {
			platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "bad request")
			return
		}

		payloadStr := r.FormValue("payload")
		if payloadStr != "" {
			h.handleInteractivePayload(r.Context(), w, payloadStr)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "bad request")
		return
	}

	ts := r.Header.Get("X-Slack-Request-Timestamp")
	sig := r.Header.Get("X-Slack-Signature")
	if !verifySlackRequest(h.signingSecret, ts, body, sig) {
		logger.Warn("Slack webhook rejected: invalid signature", "component", "webhook")
		webhookError(w, platform.ErrorCodeUnauthorized, "unauthorized")
		return
	}

	var probe slackURLVerification
	if err := json.Unmarshal(body, &probe); err != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid json")
		return
	}
	if probe.Type == "url_verification" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(probe.Challenge))
		return
	}

	if probe.Type != "event_callback" {
		respondOK(w)
		return
	}

	var env slackEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		platform.WriteErrorStatus(w, http.StatusBadRequest, platform.ErrorCodeValidationFailed, "invalid json")
		return
	}

	var ev slackMessageEvent
	if err := json.Unmarshal(env.Event, &ev); err != nil {
		respondOK(w)
		return
	}
	if ev.Type != "message" {
		respondOK(w)
		return
	}
	if ev.Subtype == "bot_message" || ev.BotID != "" {
		respondOK(w)
		return
	}
	if ev.Channel == "" || ev.Text == "" {
		respondOK(w)
		return
	}

	if h.handleIncidentCoordinationSlackMessage(r.Context(), ev) {
		respondOK(w)
		return
	}

	rootTS := ev.ThreadTS
	if rootTS == "" {
		respondOK(w)
		return
	}

	record, err := h.alertInvestigationStore.GetAlertInvestigationBySlackThread(r.Context(), ev.Channel, rootTS)
	if err != nil {
		logger.Error("Failed to query investigation by Slack thread", "component", "webhook", "error", err)
		webhookError(w, platform.ErrorCodeInternal, "internal error")
		return
	}
	if record == nil {
		respondOK(w)
		return
	}

	if alertInvestigationHasUpdateSlackTS(record, ev.TS) {
		logger.Debug("Skipping Slack webhook: post ts already recorded for investigation", "component", "webhook", "slack_ts", ev.TS, "investigation_id", record.AlertInvestigationID)
		respondOK(w)
		return
	}

	update := store.InvestigationUpdate{
		Type:      store.UpdateTypeComment,
		Message:   ev.Text,
		Source:    store.UpdateSourceSlack,
		CreatedAt: time.Now(),
	}
	if isInternal, cleaned := parseInternalNote(ev.Text); isInternal {
		update.Internal = true
		update.Message = cleaned
	}
	if ev.User != "" {
		u := ev.User
		update.UserID = &u
	}
	update.SlackMessageTS = ev.TS

	if err := h.alertInvestigationStore.AddAlertInvestigationUpdate(r.Context(), record.AlertInvestigationID, update); err != nil {
		logger.Error("Failed to add investigation update from Slack", "component", "webhook", "error", err)
		webhookError(w, platform.ErrorCodeInternal, "internal error")
		return
	}

	h.auditStore.Log(store.AuditInvestigationUpdated, nil, "slack:"+ev.User, "", "", true, map[string]any{
		"investigation_id": record.AlertInvestigationID,
		"update_type":      "comment",
		"source":           "slack",
		"slack_ts":         ev.TS,
	})

	h.sse.PublishInvestigationEvent("investigation_update", map[string]any{
		"alert_investigation_id": record.AlertInvestigationID,
		"update":                 update,
		"status":                 record.Status,
	})

	if h.botUserIDs[ev.User] {
		logger.Debug("Skipping agent forward: Slack message from bot user", "component", "webhook", "user_id", ev.User)
	} else if h.investigationForwarder != nil && record.AgentID != "" && ShouldForwardToAgent(ev.Text) {
		if err := h.investigationForwarder.ForwardToAgent(record.AgentID, record.AlertInvestigationID, ev.User, ev.User, ev.Text); err != nil {
			logger.Error("Failed to forward Slack message to agent", "component", "webhook", "error", err)
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *SlackWebhookHandler) handleInteractivePayload(ctx context.Context, w http.ResponseWriter, payloadStr string) {
	var payload slackInteractionPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		logger.Error("Failed to parse Slack interactive payload", "component", "webhook", "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if payload.Type != "block_actions" || len(payload.Actions) == 0 {
		respondOK(w)
		return
	}

	if h.alertStore == nil {
		logger.Warn("Slack interactive action received but alert store not configured", "component", "webhook")
		respondOK(w)
		return
	}

	action := payload.Actions[0]
	fingerprint := action.Value
	userName := payload.User.Name
	if userName == "" {
		userName = payload.User.Username
	}
	if userName == "" {
		userName = payload.User.ID
	}
	channelID := payload.Channel.ID
	messageTS := payload.Message.TS

	actor := &store.EventActor{
		UserID:      payload.User.ID,
		Username:    userName,
		DisplayName: userName,
		Source:      "slack",
	}

	switch action.ActionID {
	case "acknowledge":
		if err := h.alertStore.AcknowledgeAlert(fingerprint, actor); err != nil {
			logger.Error("Failed to acknowledge alert from Slack", "component", "webhook", "fingerprint", fingerprint, "error", err)
			h.respondInteractive(w, false, fmt.Sprintf("Failed to acknowledge: %s", err.Error()))
			return
		}
		h.auditStore.Log(store.AuditAlertAcknowledged, nil, "slack:"+payload.User.ID, "", "", true, map[string]any{
			"fingerprint": fingerprint,
		})
		logger.Info("Alert acknowledged from Slack", "component", "webhook", "fingerprint", fingerprint, "user_name", userName)
		h.updateAlertMessage(ctx, fingerprint, channelID, messageTS, "acknowledge", userName)

	case "resolve":
		if err := h.alertStore.ResolveAlertByUser(fingerprint, actor); err != nil {
			logger.Error("Failed to resolve alert from Slack", "component", "webhook", "fingerprint", fingerprint, "error", err)
			return
		}
		if h.dedupCache != nil {
			h.dedupCache.RemoveTracking(ctx, fingerprint)
		}
		h.auditStore.Log(store.AuditAlertResolved, nil, "slack:"+payload.User.ID, "", "", true, map[string]any{
			"fingerprint": fingerprint,
		})
		logger.Info("Alert resolved from Slack", "component", "webhook", "fingerprint", fingerprint, "user_name", userName)
		h.updateAlertMessage(ctx, fingerprint, channelID, messageTS, "resolve", userName)

	default:
		logger.Debug("Unknown Slack interactive action", "component", "webhook", "action_id", action.ActionID)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *SlackWebhookHandler) updateAlertMessage(ctx context.Context, fingerprint, channelID, messageTS, action, userName string) {
	record, err := h.alertStore.GetByFingerprint(fingerprint)
	if err != nil {
		logger.Error("Failed to fetch alert after Slack action", "component", "webhook", "fingerprint", fingerprint, "error", err)
		return
	}

	h.publishAlertUpdated(record)

	if h.slackClient == nil || !h.slackClient.Enabled() {
		return
	}

	alert := AlertFromStoreRecord(record)
	attachments, fallback := slack.BuildAlertAttachmentsUpdate(alert, action, userName)
	if err := h.slackClient.UpdateMessageWithAttachments(ctx, channelID, messageTS, fallback, attachments); err != nil {
		logger.Error("Failed to update Slack message for alert", "component", "webhook", "fingerprint", fingerprint, "error", err)
	}
}

func (h *SlackWebhookHandler) respondInteractive(w http.ResponseWriter, ok bool, message string) {
	resp := map[string]any{"ok": ok}
	if message != "" {
		resp["text"] = message
	}
	platform.WriteJSON(w, http.StatusOK, resp)
}

func respondOK(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func alertInvestigationHasUpdateSlackTS(record *store.AlertInvestigationRecord, ts string) bool {
	return alertInvestigationUpdateContains(record, func(u store.InvestigationUpdate) bool {
		return u.SlackMessageTS == ts
	})
}

func alertInvestigationHasUpdateMMPostID(record *store.AlertInvestigationRecord, postID string) bool {
	return alertInvestigationUpdateContains(record, func(u store.InvestigationUpdate) bool {
		return u.MMPostID == postID
	})
}

func alertInvestigationUpdateContains(record *store.AlertInvestigationRecord, match func(store.InvestigationUpdate) bool) bool {
	if record == nil {
		return false
	}
	for _, u := range record.Updates {
		if match(u) {
			return true
		}
	}
	return false
}

func verifySlackRequest(signingSecret, timestamp string, body []byte, wantSig string) bool {
	if signingSecret == "" || timestamp == "" || wantSig == "" {
		return false
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if delta := time.Since(time.Unix(ts, 0)); delta > 5*time.Minute || delta < -5*time.Minute {
		return false
	}
	base := "v0:" + timestamp + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(wantSig))
}
