package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"alga/api/platform"
	"alga/logger"
	"alga/mattermost"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/routing"
	"alga/slack"
	"alga/sse"
	"alga/store"
	"alga/types"
	"alga/valkey"
)

// DedupCache is the interface for alert deduplication checking.
type DedupCache interface {
	IsDuplicate(ctx context.Context, fingerprint string) bool
	MarkTracked(ctx context.Context, fingerprint string) error
	RemoveTracking(ctx context.Context, fingerprint string)
}

// AlertPublisher publishes alerts for async processing.
type AlertPublisher interface {
	PublishAlert(ctx context.Context, payload types.GrafanaAlertingPayload) error
}

// InvestigatePublisher publishes investigate messages for SRE agent processing.
type InvestigatePublisher interface {
	PublishInvestigation(ctx context.Context, msg rabbitmq.InvestigateMessage) error
}

// Correlator triggers alert correlation for SRE agent investigation.
type Correlator interface {
	ProcessAlert(ctx context.Context, alert rabbitmq.CorrelatedAlert) error
}

// RateLimiter guards public webhook ingress against volumetric abuse of
// pre-auth work (token lookups, HMAC verification). Implementations must be
// safe for concurrent use; Allow returns true when the caller is under the
// configured limit. Matches api.RateLimiting without creating an import cycle.
type RateLimiter interface {
	Allow(key string) bool
}

// Receiver handles incoming webhook requests
type Receiver struct {
	mu                          sync.RWMutex
	routingEngine               *routing.Engine
	mmClient                    *mattermost.Client
	slackClient                 *slack.Client
	chatRouter                  *ChatRouter
	store                       store.Store
	webhookTokenStore           store.WebhookTokenStore
	dedupCache                  DedupCache
	rateLimiter                 RateLimiter
	publisher                   AlertPublisher
	outboxStore                 store.OutboxStore
	eventPublisher              store.AlertEventPublisher
	correlator                  Correlator
	alertInvestigationStore     store.AlertInvestigationStore
	incidentInvestigationStore  store.IncidentInvestigationStore
	investigationForwarder      InvestigationAgentForwarder
	pendingNotifier             PendingNotifier
	maintenanceStore            store.MaintenanceWindowStore
	auditStore                  store.AuditStore
	sse                         SSEPublisherMixin
	alertInvestigationLifecycle alertInvestigationLifecycle
	idempotency                 *valkey.IdempotencyCache
	idempotencyTTL              time.Duration
}

type alertInvestigationLifecycle interface {
	CompleteIfAllAlertsResolved(ctx context.Context, req store.AlertInvestigationLifecycleCompletionRequest) error
}

type PendingNotifier interface {
	NotifyPending()
}

// NewReceiver creates a new webhook receiver
func NewReceiver(routingEngine *routing.Engine, mmClient *mattermost.Client, slackClient *slack.Client, alertStore store.Store, webhookTokenStore store.WebhookTokenStore, dedupCache DedupCache) *Receiver {
	var providers []ChatProvider
	if mmClient != nil {
		providers = append(providers, NewMattermostChatProvider(mmClient))
	}
	if slackClient != nil {
		providers = append(providers, NewSlackChatProvider(slackClient))
	}
	return &Receiver{
		routingEngine:     routingEngine,
		mmClient:          mmClient,
		slackClient:       slackClient,
		chatRouter:        NewChatRouter(providers...),
		store:             alertStore,
		webhookTokenStore: webhookTokenStore,
		dedupCache:        dedupCache,
	}
}

// SetPublisher enables async processing via message queue.
func (r *Receiver) SetPublisher(p AlertPublisher) {
	r.publisher = p
}

// SetOutboxStore enables the transactional outbox: instead of publishing the
// alert directly, the hot path writes a durable outbox row that the outbox
// publisher worker later flushes to RabbitMQ. This matches the W6 outbox
// pattern and prevents lost events when the broker is unavailable.
func (r *Receiver) SetOutboxStore(s store.OutboxStore) {
	r.outboxStore = s
}

// SetRateLimiter guards webhook ingress against volumetric abuse of the
// pre-auth work (token validation) performed on every request.
func (r *Receiver) SetRateLimiter(rl RateLimiter) {
	r.rateLimiter = rl
}

// SetIdempotency enables Idempotency-Key replay for the alert-ingest webhook.
// The receiver already dedups alerts by fingerprint; this adds HTTP-level
// replay so a client that retries an entire request (e.g. after a network
// timeout) receives the original response instead of re-running the handler.
// A nil cache leaves the route unwrapped (pass-through). Must be called before
// Router() so the wrapper is applied at registration time.
func (r *Receiver) SetIdempotency(cache *valkey.IdempotencyCache, ttl time.Duration) {
	r.idempotency = cache
	r.idempotencyTTL = ttl
}

// SetEventPublisher enables real-time SSE event publishing for alert changes.
func (r *Receiver) SetEventPublisher(p store.AlertEventPublisher) {
	r.eventPublisher = p
}

// SetCorrelator enables SRE agent alert correlation.
func (r *Receiver) SetCorrelator(c Correlator) {
	r.correlator = c
}

// SetInvestigationStore sets the investigation store for reopening linked
// investigations on alert refire.
func (r *Receiver) SetAlertInvestigationStore(s store.AlertInvestigationStore) {
	r.alertInvestigationStore = s
}

func (r *Receiver) SetIncidentInvestigationStore(s store.IncidentInvestigationStore) {
	r.incidentInvestigationStore = s
}

// SetInvestigationForwarder sets the forwarder for sending signals to agents.
func (r *Receiver) SetInvestigationForwarder(f InvestigationAgentForwarder) {
	r.investigationForwarder = f
}

func (r *Receiver) SetPendingNotifier(n PendingNotifier) {
	r.pendingNotifier = n
}

func (r *Receiver) SetMaintenanceStore(s store.MaintenanceWindowStore) {
	r.maintenanceStore = s
}

func (r *Receiver) SetAuditStore(s store.AuditStore) {
	r.auditStore = s
}

func (r *Receiver) SetAlertInvestigationLifecycleService(svc alertInvestigationLifecycle) {
	r.alertInvestigationLifecycle = svc
}

func (r *Receiver) SetSSEBroker(broker *sse.Broker, vkClient *valkey.Client) {
	r.sse.SetSSEBroker(broker, vkClient)
}

// SetRoutingEngine replaces the routing engine at runtime.
func (r *Receiver) SetRoutingEngine(engine *routing.Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routingEngine = engine
}

// ChatRouter returns the provider router for external callers that need chat dispatch.
func (r *Receiver) ChatRouter() *ChatRouter {
	return r.chatRouter
}

func (r *Receiver) route(alert types.Alert) routing.RouteResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.routingEngine.Route(alert)
}

// Router returns the HTTP handler for the webhook receiver
func (r *Receiver) Router() *http.ServeMux {
	mux := http.NewServeMux()
	alertHandler := r.handleWebhook
	if r.idempotency != nil {
		alertHandler = platform.WithIdempotency(r.idempotency, r.idempotencyTTL, "alert:ingest", r.handleWebhook)
	}
	mux.HandleFunc("/webhooks/alerts", alertHandler)
	return mux
}

// handleWebhook processes incoming webhook alerts
func (r *Receiver) handleWebhook(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.rateLimiter != nil && !r.rateLimiter.Allow(clientIPFromRequest(req)) {
		platform.WriteRateLimitExceeded(w, "60")
		return
	}

	token := webhookTokenFromRequest(req)
	if token == "" {
		http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
		return
	}

	valid, err := r.webhookTokenStore.ValidateToken(token)
	if err != nil {
		logger.Error("Failed to validate webhook token", "component", "webhook", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !valid {
		http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
		return
	}

	// Limit request body size to prevent abuse
	req.Body = http.MaxBytesReader(w, req.Body, platform.MaxRequestBodySize) // 1 MB max

	var payload types.GrafanaAlertingPayload

	body, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Error("Failed to read webhook request body", "component", "webhook", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		logger.Error("Failed to decode webhook payload", "component", "webhook", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	logger.Debug("Received webhook payload", "component", "webhook", "size", len(body), "alert_count", len(payload.Alerts))

	logger.Info("Received webhook with alerts", "component", "webhook", "alert_count", len(payload.Alerts))

	// If async publisher is set, publish and return 202 Accepted.
	// Preferred path (W6): write a durable outbox row and let the outbox
	// publisher worker flush it to RabbitMQ. This makes event publication
	// reliable — if the broker is down, the row is replayed later instead of
	// being lost. Falls back to the legacy direct publish (and then to sync
	// processing) only when the outbox is unavailable or errors.
	if r.publisher != nil {
		enqueued := false
		if r.outboxStore != nil {
			body, eventID, mErr := rabbitmq.MarshalAlertMessage(req.Context(), payload)
			if mErr != nil {
				logger.Error("outbox: failed to marshal alert message; falling back to direct publish", "component", "webhook", "error", mErr)
			} else {
				aggregateID := ""
				if len(payload.Alerts) > 0 {
					aggregateID = payload.Alerts[0].Fingerprint
				}
				if qErr := r.outboxStore.EnqueueOutbox(req.Context(), rabbitmq.EventTypeAlertReceived, aggregateID, rabbitmq.ExchangeAlerts, rabbitmq.RoutingKeyAlertProcess, body, eventID); qErr != nil {
					logger.Warn("outbox: enqueue failed; falling back to direct publish", "component", "webhook", "alert_count", len(payload.Alerts), "error", qErr)
				} else {
					enqueued = true
				}
			}
		}
		if enqueued {
			metrics.WebhookAlertPublishQueued.Add(1)
			platform.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
			return
		}
		// Legacy direct-publish path (kept for non-migrated deployments).
		if err := r.publisher.PublishAlert(req.Context(), payload); err != nil {
			metrics.WebhookAlertPublishSyncFallback.Add(1)
			logger.Warn("Webhook async publish failed, falling back to sync", "component", "webhook", "alert_count", len(payload.Alerts), "error", err)
			// Fall through to synchronous processing
		} else {
			metrics.WebhookAlertPublishQueued.Add(1)
			platform.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
			return
		}
	}

	metrics.WebhookAlertPublishSyncProcessed.Add(1)
	if err := r.ProcessAlerts(req.Context(), payload); err != nil {
		logger.Error("Sync alert processing failed", "component", "webhook", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	platform.WriteStatus(w, "ok")
}

func webhookTokenFromRequest(req *http.Request) string {
	token := strings.TrimSpace(req.Header.Get("Authorization"))
	if token != "" {
		if len(token) > 7 && strings.EqualFold(token[:7], "Bearer ") {
			return strings.TrimSpace(token[7:])
		}
		return token
	}
	return strings.TrimSpace(req.URL.Query().Get("token"))
}

// clientIPFromRequest returns a best-effort client IP for rate limiting.
// It honors the first X-Forwarded-For entry (set by trusted front proxies)
// and falls back to the connection's remote address.
func clientIPFromRequest(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host := req.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return strings.Trim(host, "[]")
}

// ProcessAlerts processes all alerts in a payload. Used by both sync and async paths.
// Returns the first transient error encountered so callers (e.g. AlertWorker) can
// requeue the entire payload for retry. Individual alerts that fail are logged and
// skipped; already-processed alerts are caught on retry via the dedup cache.
func (r *Receiver) ProcessAlerts(ctx context.Context, payload types.GrafanaAlertingPayload) error {
	var firstErr error
	for _, alert := range payload.Alerts {
		if r.dedupCache != nil {
			if alert.Status != "resolved" && r.dedupCache.IsDuplicate(ctx, alert.Fingerprint) {
				logger.Debug("Alert deduplicated via cache, skipping", "component", "webhook", "fingerprint", alert.Fingerprint)
				continue
			}
		}

		existing, err := r.store.GetOpenByFingerprint(alert.Fingerprint)
		if err != nil {
			logger.Error("Failed to query alert store for fingerprint", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		if alert.Status == "resolved" {
			r.handleResolved(ctx, alert, existing)
			continue
		}

		r.handleFiring(ctx, alert, existing)
	}
	return firstErr
}

func parseStartsAt(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func (r *Receiver) deliverToDestinations(ctx context.Context, alert types.Alert, destinations []routing.Destination) []store.DeliveryTarget {
	var targets []store.DeliveryTarget
	for _, dest := range destinations {
		postID, resolvedChannel, err := r.sendAlert(ctx, dest, alert)
		if err != nil {
			logger.Error("Failed to create post for alert", "component", "webhook", "alert_name", alert.Labels["alertname"], "error", err)
			continue
		}
		ch := resolvedChannel
		if ch == "" {
			ch = dest.Channel
		}
		targets = append(targets, store.DeliveryTarget{
			Provider:    dest.Provider,
			Channel:     ch,
			ChannelName: dest.Channel,
			PostID:      postID,
		})
	}
	return targets
}

func (r *Receiver) handleFiring(ctx context.Context, alert types.Alert, existing *store.AlertRecord) {
	if r.isSuppressed(ctx, alert.Labels) {
		if existing != nil {
			logger.Debug("Alert already tracked and suppressed, skipping", "component", "webhook", "fingerprint", alert.Fingerprint)
			return
		}

		startsAt := parseStartsAt(alert.StartsAt)
		record := store.AlertRecord{
			Fingerprint:  alert.Fingerprint,
			Status:       "firing",
			Silenced:     true,
			Labels:       alert.Labels,
			Annotations:  alert.Annotations,
			Values:       alert.Values,
			StartsAt:     startsAt,
			GeneratorURL: alert.GeneratorURL,
		}

		logger.Info("Alert suppressed by maintenance window, storing without notification", "component", "webhook", "alert_name", alert.Labels["alertname"])
		if _, err := r.store.Create(record); err != nil {
			logger.Error("Failed to store suppressed alert record", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
			return
		}
		if r.dedupCache != nil {
			_ = r.dedupCache.MarkTracked(ctx, alert.Fingerprint)
		}
		if r.eventPublisher != nil {
			r.eventPublisher.PublishAlertEvent("alert_created", record)
		}
		return
	}

	result := r.route(alert)

	if existing != nil {
		logger.Debug("Alert already tracked, skipping", "component", "webhook", "fingerprint", alert.Fingerprint, "target_count", len(existing.DeliveryTargets))
		return
	}

	startsAt := parseStartsAt(alert.StartsAt)

	record := store.AlertRecord{
		Fingerprint:  alert.Fingerprint,
		Status:       "firing",
		Silenced:     result.Silenced,
		Labels:       alert.Labels,
		Annotations:  alert.Annotations,
		Values:       alert.Values,
		StartsAt:     startsAt,
		GeneratorURL: alert.GeneratorURL,
	}

	if result.Silenced {
		logger.Info("Alert silenced, storing without notification", "component", "webhook", "alert_name", alert.Labels["alertname"])
		alertNum, err := r.store.Create(record)
		if err != nil {
			logger.Error("Failed to store silenced alert record", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
			return
		}
		record.AlertNumber = alertNum
		if r.dedupCache != nil {
			_ = r.dedupCache.MarkTracked(ctx, alert.Fingerprint)
		}
		if r.eventPublisher != nil {
			r.eventPublisher.PublishAlertEvent("alert_created", record)
		}
		r.triggerCorrelator(ctx, alert, alertNum)
		return
	}

	if len(result.Destinations) == 0 {
		name := alert.Labels["alertname"]
		if name == "" {
			name = alert.Fingerprint
		}
		logger.Info("Routing produced no destinations for alert, persisting without chat delivery", "component", "webhook", "alert_name", name, "fingerprint", alert.Fingerprint)
		alertNum, err := r.store.Create(record)
		if err != nil {
			logger.Error("Failed to store alert record", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
		} else {
			record.AlertNumber = alertNum
			if r.dedupCache != nil {
				_ = r.dedupCache.MarkTracked(ctx, alert.Fingerprint)
			}
			if r.eventPublisher != nil {
				r.eventPublisher.PublishAlertEvent("alert_created", record)
			}
		}
		r.triggerCorrelator(ctx, alert, alertNum)
		return
	}

	alertNum, storeErr := r.store.Create(record)
	if storeErr != nil {
		logger.Error("Failed to store alert record", "component", "webhook", "fingerprint", alert.Fingerprint, "error", storeErr)
	} else {
		record.AlertNumber = alertNum
	}

	if storeErr == nil && r.dedupCache != nil {
		_ = r.dedupCache.MarkTracked(ctx, alert.Fingerprint)
	}

	if storeErr != nil {
		r.triggerCorrelator(ctx, alert, 0)
		return
	}

	targets := r.deliverToDestinations(ctx, alert, result.Destinations)
	if len(targets) == 0 {
		name := alert.Labels["alertname"]
		if name == "" {
			name = alert.Fingerprint
		}
		logger.Warn("All delivery targets failed for alert, continuing without chat posts", "component", "webhook", "alert_name", name, "fingerprint", alert.Fingerprint)
		if storeErr == nil {
			if err := r.store.UpdateDeliveryTargets(alert.Fingerprint, nil); err != nil {
				logger.Error("Failed to clear delivery targets for alert", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
			}
			if r.eventPublisher != nil {
				r.eventPublisher.PublishAlertEvent("alert_created", record)
			}
		}
		r.triggerCorrelator(ctx, alert, alertNum)
		return
	}

	if storeErr == nil {
		if err := r.store.UpdateDeliveryTargets(alert.Fingerprint, targets); err != nil {
			logger.Error("Failed to update delivery targets for alert", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
		}
	}

	record.DeliveryTargets = targets
	if r.eventPublisher != nil && storeErr == nil {
		r.eventPublisher.PublishAlertEvent("alert_created", record)
	}

	r.triggerCorrelator(ctx, alert, alertNum)

	for _, t := range targets {
		logger.Info("Created post for alert", "component", "webhook", "post_id", t.PostID, "alert_name", alert.Labels["alertname"], "provider", t.Provider, "channel", t.Channel)
	}
}

func (r *Receiver) handleResolved(ctx context.Context, alert types.Alert, existing *store.AlertRecord) {
	result := r.route(alert)

	startsAt := parseStartsAt(alert.StartsAt)

	suppressed := r.isSuppressed(ctx, alert.Labels)

	if existing == nil {
		// No open alert. There may still be an already-resolved record for this
		// fingerprint (e.g. a duplicate or late resolved notification). Resolved
		// alerts are never auto-reopened, and a resolved notification must be
		// idempotent: do not spawn a phantom new alert record.
		latest, err := r.store.GetByFingerprint(alert.Fingerprint)
		if err != nil {
			logger.Error("Failed to look up alert by fingerprint for resolved dedup", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
			return
		}
		if latest != nil && latest.Status == "resolved" {
			logger.Debug("Received resolved notification for already-resolved fingerprint, skipping", "component", "webhook", "fingerprint", alert.Fingerprint)
			return
		}

		logger.Warn("Received resolved alert with no tracking record, creating new record", "component", "webhook", "fingerprint", alert.Fingerprint)

		record := store.AlertRecord{
			Fingerprint:  alert.Fingerprint,
			Status:       "resolved",
			Silenced:     result.Silenced || suppressed,
			Labels:       alert.Labels,
			Annotations:  alert.Annotations,
			Values:       alert.Values,
			StartsAt:     startsAt,
			GeneratorURL: alert.GeneratorURL,
		}

		if _, err := r.store.Create(record); err != nil {
			logger.Error("Failed to store resolved alert record", "component", "webhook", "error", err)
			return
		}

		if result.Silenced || suppressed {
			logger.Info("Alert silenced/suppressed, not routing resolved event", "component", "webhook", "alert_name", alert.Labels["alertname"])
			if r.eventPublisher != nil {
				r.eventPublisher.PublishAlertEvent("alert_created", record)
			}
			return
		}

		if len(result.Destinations) == 0 {
			return
		}

		targets := r.deliverToDestinations(ctx, alert, result.Destinations)
		if len(targets) == 0 {
			return
		}

		if err := r.store.UpdateDeliveryTargets(alert.Fingerprint, targets); err != nil {
			logger.Error("Failed to update post IDs for resolved alert", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
		}

		record.DeliveryTargets = targets
		if r.eventPublisher != nil {
			r.eventPublisher.PublishAlertEvent("alert_created", record)
		}

		logger.Info("Created resolved posts for alert", "component", "webhook", "alert_name", alert.Labels["alertname"])
		return
	}

	if existing.Status == "resolved" {
		logger.Debug("Alert already resolved, skipping", "component", "webhook", "fingerprint", alert.Fingerprint)
		return
	}

	if result.Silenced {
		logger.Info("Alert silenced, marking as resolved without updating post", "component", "webhook", "alert_name", alert.Labels["alertname"])
		if err := r.store.UpdateStatusSilenced(alert.Fingerprint); err != nil {
			logger.Error("Failed to update silenced status in store", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
		} else {
			if r.dedupCache != nil {
				r.dedupCache.RemoveTracking(ctx, alert.Fingerprint)
			}
			if r.eventPublisher != nil {
				updated := *existing
				updated.Status = "resolved"
				updated.Silenced = true
				updated.UpdatedAt = time.Now()
				r.eventPublisher.PublishAlertEvent("alert_updated", updated)
			}
		}
		r.handleAutoResolvedInvestigation(ctx, alert, existing.AlertNumber)
		return
	}

	if err := r.store.UpdateStatus(alert.Fingerprint, "resolved", nil); err != nil {
		logger.Error("Failed to update alert status in store", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
		return
	}

	if r.auditStore != nil {
		r.auditStore.Log(store.AuditAlertResolved, nil, "grafana:monitoring", "", "", true, map[string]any{
			"fingerprint": alert.Fingerprint,
		})
	}

	if r.dedupCache != nil {
		r.dedupCache.RemoveTracking(ctx, alert.Fingerprint)
	}

	if err := r.updateAllAlertTargets(ctx, existing, alert); err != nil {
		logger.Error("Failed to update posts for resolved alert", "component", "webhook", "error", err)
		return
	}

	if r.eventPublisher != nil {
		updated := *existing
		updated.Status = "resolved"
		updated.UpdatedAt = time.Now()
		r.eventPublisher.PublishAlertEvent("alert_updated", updated)
	}

	logger.Info("Updated posts to resolved for alert", "component", "webhook", "alert_name", alert.Labels["alertname"])

	r.handleAutoResolvedInvestigation(ctx, alert, existing.AlertNumber)
}

func (r *Receiver) handleAutoResolvedInvestigation(ctx context.Context, alert types.Alert, alertNumber int64) {
	if r.alertInvestigationStore == nil || alertNumber == 0 {
		return
	}

	if r.alertInvestigationLifecycle != nil {
		if err := r.alertInvestigationLifecycle.CompleteIfAllAlertsResolved(ctx, store.AlertInvestigationLifecycleCompletionRequest{
			AlertNumber: alertNumber,
			Reason:      store.AlertInvestigationCompletedReasonMonitoringResolved,
			ActorType:   store.InvestigationActorGrafana,
			ActorName:   "grafana",
		}); err != nil {
			logger.Warn("Auto-complete: lifecycle service failed for resolved alert", "component", "webhook", "alert_number", alertNumber, "error", err)
		}
	}

	alertName := alert.Labels["alertname"]
	if alertName == "" {
		alertName = alert.Fingerprint
	}

	linked, err := r.alertInvestigationStore.ListAlertInvestigationsByAlertNumber(ctx, alertNumber)
	if err != nil {
		return
	}

	for i := range linked {
		inv := &linked[i]
		r.completePromotedIncidentInvestigationIfResolved(ctx, inv, alertName)
		if inv.AgentID != "" && r.investigationForwarder != nil {
			event := sse.Event{
				Type: "alert_auto_resolved",
				Data: map[string]any{
					"investigation_id": inv.AlertInvestigationID,
					"fingerprint":      alert.Fingerprint,
					"alert_name":       alertName,
				},
			}
			_ = r.investigationForwarder.ForwardEventToAgent(inv.AgentID, event)
		}
	}
}

func (r *Receiver) completePromotedIncidentInvestigationIfResolved(ctx context.Context, inv *store.AlertInvestigationRecord, alertName string) {
	if r.incidentInvestigationStore == nil || inv == nil || inv.PromotedIncidentInvestigationID == nil || !r.allAlertInvestigationAlertsResolved(inv) {
		return
	}
	incidentInvestigationID := inv.PromotedIncidentInvestigationID.String()
	incidentInv, err := r.incidentInvestigationStore.GetIncidentInvestigation(ctx, incidentInvestigationID)
	if err != nil || incidentInv == nil || !isActiveIncidentInvestigationStatus(incidentInv.Status) {
		return
	}
	message := "Grafana resolved all alerts linked to the promoted alert investigation. Completing incident investigation."
	if alertName != "" {
		message = fmt.Sprintf("Grafana resolved %s and all alerts linked to the promoted alert investigation. Completing incident investigation.", alertName)
	}
	if err := r.incidentInvestigationStore.AddIncidentInvestigationUpdate(ctx, incidentInvestigationID, store.InvestigationUpdate{
		Type:    store.UpdateTypeProgress,
		Message: message,
		Source:  store.UpdateSourceSystem,
	}); err != nil {
		logger.WarnCtx(ctx, "failed to add incident investigation auto-resolve update", "incident_investigation_id", incidentInvestigationID, "error", err)
	}
	if err := r.incidentInvestigationStore.UpdateIncidentInvestigationStatus(ctx, incidentInvestigationID, store.IncidentInvestigationStatusComplete); err != nil {
		logger.WarnCtx(ctx, "failed to complete promoted incident investigation after alert auto-resolve", "incident_investigation_id", incidentInvestigationID, "error", err)
		return
	}
	if r.auditStore != nil {
		r.auditStore.Log(store.AuditInvestigationUpdated, nil, "grafana:monitoring", "", "", true, map[string]any{
			"investigation_id": incidentInvestigationID,
			"status":           store.IncidentInvestigationStatusComplete,
			"reason":           "auto_resolved_on_alert_completion",
			"alert_name":       alertName,
		})
	}
	r.sse.PublishInvestigationEvent("investigation_status_changed", map[string]string{"investigation_id": incidentInvestigationID, "status": store.IncidentInvestigationStatusComplete})
	if incidentInv.AgentID != "" && r.investigationForwarder != nil {
		_ = r.investigationForwarder.ForwardEventToAgent(incidentInv.AgentID, sse.Event{Type: "alert_auto_resolved", Data: map[string]any{"investigation_id": incidentInvestigationID, "alert_name": alertName}})
	}
}

func isActiveIncidentInvestigationStatus(status string) bool {
	switch status {
	case store.IncidentInvestigationStatusPending, store.IncidentInvestigationStatusAssigned, store.IncidentInvestigationStatusInvestigating, store.IncidentInvestigationStatusPaused:
		return true
	default:
		return false
	}
}

func (r *Receiver) allAlertInvestigationAlertsResolved(inv *store.AlertInvestigationRecord) bool {
	if r.store == nil {
		return false
	}
	return store.AllAlertInvestigationAlertsResolved(r.store, inv)
}

func (r *Receiver) sendAlert(ctx context.Context, destination routing.Destination, alert types.Alert) (string, string, error) {
	return r.chatRouter.SendAlert(ctx, destination.Provider, destination.Channel, alert)
}

func (r *Receiver) updateAllAlertTargets(ctx context.Context, existing *store.AlertRecord, alert types.Alert) error {
	return UpdateChatPostsForAlert(ctx, r.chatRouter, existing, alert)
}

// IngestManualAlert stores a user-authored alert and runs the same routing,
// delivery, SSE, and correlator side-effects as a fresh Grafana firing alert.
// The initial history event is attributed to `actor` so the alert detail page
// shows "fired by <display name>" instead of "fired" with source=grafana.
//
// The caller is expected to have generated a unique fingerprint (e.g. "manual-<uuid>").
func (r *Receiver) IngestManualAlert(ctx context.Context, alert types.Alert, actor *store.EventActor) (*store.AlertRecord, error) {
	existing, err := r.store.GetOpenByFingerprint(alert.Fingerprint)
	if err != nil {
		return nil, fmt.Errorf("failed to query alert store: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("%w: %s", store.ErrOpenAlertExists, alert.Fingerprint)
	}

	startsAt := parseStartsAt(alert.StartsAt)
	if startsAt.IsZero() {
		startsAt = time.Now().UTC()
	}

	result := r.route(alert)
	firedEv := store.AlertEventWithActor("fired", startsAt, actor)

	record := store.AlertRecord{
		Fingerprint:  alert.Fingerprint,
		Status:       "firing",
		Silenced:     result.Silenced,
		Labels:       alert.Labels,
		Annotations:  alert.Annotations,
		Values:       alert.Values,
		StartsAt:     startsAt,
		GeneratorURL: alert.GeneratorURL,
		InitialEvent: &firedEv,
	}

	if alertNum, err := r.store.Create(record); err != nil {
		return nil, fmt.Errorf("failed to store alert record: %w", err)
	} else {
		record.AlertNumber = alertNum
	}

	if r.dedupCache != nil {
		if err := r.dedupCache.MarkTracked(ctx, alert.Fingerprint); err != nil {
			logger.Error("Failed to mark manual alert in dedup cache", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
		}
	}

	if !result.Silenced && len(result.Destinations) > 0 {
		targets := r.deliverToDestinations(ctx, alert, result.Destinations)
		if len(targets) > 0 {
			if err := r.store.UpdateDeliveryTargets(alert.Fingerprint, targets); err != nil {
				logger.Error("Failed to update delivery targets for manual alert", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
			}
		}
	}

	rec, err := r.store.GetByFingerprint(alert.Fingerprint)
	if err != nil {
		return nil, fmt.Errorf("failed to reload alert after create: %w", err)
	}
	if rec == nil {
		return nil, fmt.Errorf("alert disappeared after create: %s", alert.Fingerprint)
	}

	if r.eventPublisher != nil {
		r.eventPublisher.PublishAlertEvent("alert_created", *rec)
	}

	r.triggerCorrelator(ctx, alert, rec.AlertNumber)

	return rec, nil
}

// HandleHealth returns a simple health check
func (r *Receiver) HandleHealth(w http.ResponseWriter, req *http.Request) {
	platform.WriteStatus(w, "ok")
}

func (r *Receiver) triggerCorrelator(ctx context.Context, alert types.Alert, alertNumber int64) {
	if r.correlator == nil {
		return
	}

	values := make(map[string]float64)
	for k, v := range alert.Values {
		if f, ok := v.(float64); ok {
			values[k] = f
		}
	}

	correlatedAlert := rabbitmq.CorrelatedAlert{
		Fingerprint:  alert.Fingerprint,
		AlertNumber:  alertNumber,
		Labels:       alert.Labels,
		Annotations:  alert.Annotations,
		Status:       alert.Status,
		StartsAt:     alert.StartsAt,
		Values:       values,
		GeneratorURL: alert.GeneratorURL,
	}

	if err := r.correlator.ProcessAlert(ctx, correlatedAlert); err != nil {
		logger.Error("Failed to trigger correlator for alert", "component", "webhook", "fingerprint", alert.Fingerprint, "error", err)
	}
}

func (r *Receiver) isSuppressed(ctx context.Context, labels map[string]string) bool {
	if r.maintenanceStore == nil {
		return false
	}
	windows, err := r.maintenanceStore.ListActive(ctx)
	if err != nil {
		return false
	}
	for _, w := range windows {
		if matchLabels(w.LabelMatchers, labels) {
			return true
		}
	}
	return false
}

func matchLabels(matchers, labels map[string]string) bool {
	if len(matchers) == 0 {
		return false
	}
	for k, v := range matchers {
		if labels[k] != v {
			return false
		}
	}
	return true
}
