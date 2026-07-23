package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"alga/cancellation"
	"alga/logger"
	"alga/mattermost"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/slack"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

// dedupeKeyTTL is how long a per-message idempotency token stays in Valkey
// after a successful processInvestigation. Long enough to absorb broker
// re-deliveries and replica fail-over, short enough that DedupeKey reuse
// (e.g. correlator restart) doesn't accidentally mask a brand new flush.
const dedupeKeyTTL = 24 * time.Hour

func dedupeKey(msg rabbitmq.InvestigateMessage) string {
	if msg.DedupeKey != "" {
		return "alga:investigate:dedupe:" + msg.DedupeKey
	}
	if msg.InvestigationID != "" {
		return "alga:investigate:dedupe:inv:" + msg.InvestigationID
	}
	return ""
}

type InvestigationNotifier interface {
	NotifyPending()
}

type InvestigateWorker struct {
	mmClient                *mattermost.Client
	slackClient             *slack.Client
	alertStore              store.Store
	alertInvestigationStore store.AlertInvestigationStore
	sseBroker               *sse.Broker
	ssePublisher            *sse.DualPublisher
	vkClient                *valkey.Client
	cancelSet               *valkey.CancelSet
	publisher               *rabbitmq.Publisher
	cfg                     InvestigateConfig
	wg                      sync.WaitGroup
	notifier                InvestigationNotifier
}

type InvestigateConfig struct {
	InvestigationTimeout        time.Duration
	InvestigationChannel        string
	MaxConcurrentInvestigations int
	CriticalSeverityLabels      []string
}

func (w *InvestigateWorker) SetNotifier(n InvestigationNotifier) {
	w.notifier = n
}

func (w *InvestigateWorker) SetCancelSet(cs *valkey.CancelSet) { w.cancelSet = cs }

func (w *InvestigateWorker) SetValkeyClient(c *valkey.Client) {
	w.vkClient = c
	if w.sseBroker != nil {
		w.ssePublisher = &sse.DualPublisher{Broker: w.sseBroker, VKClient: c}
	}
}

// SetPublisher wires the rabbitmq publisher used to schedule retry attempts
// (alga.investigate.retry.{1,2,3}). When nil, a transient processing error
// dead-letters via Nack(requeue=false) instead of being retried.
func (w *InvestigateWorker) SetPublisher(p *rabbitmq.Publisher) {
	w.publisher = p
}

func NewInvestigateWorker(
	mmClient *mattermost.Client,
	slackClient *slack.Client,
	alertStore store.Store,
	alertInvestigationStore store.AlertInvestigationStore,
	sseBroker *sse.Broker,
	cfg InvestigateConfig,
) *InvestigateWorker {
	if cfg.MaxConcurrentInvestigations < 1 {
		cfg.MaxConcurrentInvestigations = 1
	}
	return &InvestigateWorker{
		mmClient:                mmClient,
		slackClient:             slackClient,
		alertStore:              alertStore,
		alertInvestigationStore: alertInvestigationStore,
		sseBroker:               sseBroker,
		cfg:                     cfg,
	}
}

func (w *InvestigateWorker) Queue() string {
	return rabbitmq.QueueInvestigateProcess
}

func (w *InvestigateWorker) Handle(ctx context.Context, d amqp.Delivery) {
	w.wg.Add(1)
	defer w.wg.Done()
	w.processInvestigation(ctx, d)
}

// PrefetchCount caps how many unacked investigations RabbitMQ delivers per
// consumer. We anchor it to MaxConcurrentInvestigations so a single replica
// never works on more in-flight investigations than the operator allowed.
func (w *InvestigateWorker) PrefetchCount() int {
	return w.cfg.MaxConcurrentInvestigations
}

// Stop blocks until all in-flight processInvestigation calls return so a
// graceful shutdown does not strand investigations mid-creation.
func (w *InvestigateWorker) Stop() {
	w.wg.Wait()
}

func (w *InvestigateWorker) processInvestigation(ctx context.Context, d amqp.Delivery) {
	var msg rabbitmq.InvestigateMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error("Failed to unmarshal investigate message", "component", "investigate-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	traceID := msg.TraceID
	if traceID == "" {
		traceID = msg.CorrelationKey
	}
	logger.Info("Processing investigation", "component", "investigate-worker", "investigation_id", msg.InvestigationID, "alert_count", len(msg.Alerts), "severity", msg.Severity, "trace_id", traceID, "retry", msg.RetryCount)

	// Guard: ack+drop retries whose target entity was deleted. The hard-deleted
	// investigation / soft-deleted alert makes these jobs stale; the cancel set
	// is the fast path, the PG deleted_at check is the durable backstop. Without
	// this, a retry for a hard-deleted investigation would recreate stale work.
	if msg.InvestigationID != "" && cancellation.InvestigationCancelled(ctx, w.cancelSet, msg.InvestigationID) {
		logger.Info("Dropping investigation job; investigation cancelled", "component", "investigate-worker", "investigation_id", msg.InvestigationID)
		_ = d.Ack(false)
		return
	}
	primaryFP := msg.PrimaryAlertFingerprint
	if primaryFP == "" && len(msg.Alerts) > 0 {
		primaryFP = msg.Alerts[0].Fingerprint
	}
	if cancellation.AlertCancelled(ctx, w.cancelSet, w.alertStore, primaryFP, msg.PrimaryAlertNumber) {
		logger.Info("Dropping investigation job; primary alert deleted", "component", "investigate-worker", "fingerprint", primaryFP)
		_ = d.Ack(false)
		return
	}

	// Idempotency layer 1: Valkey SETNX on a per-message dedupe key.
	// Catches re-deliveries that arrive before the PostgreSQL unique index would
	// reject the duplicate (which can happen when the previous attempt
	// crashed *between* insert and Ack).
	if w.vkClient != nil {
		if dk := dedupeKey(msg); dk != "" {
			ok, err := w.vkClient.SetNX(ctx, dk, "1", dedupeKeyTTL)
			if err != nil {
				logger.Warn("Dedupe SETNX failed; continuing", "component", "investigate-worker", "investigation_id", msg.InvestigationID, "error", err)
			} else if !ok {
				logger.Info("Dropping duplicate investigation delivery", "component", "investigate-worker", "investigation_id", msg.InvestigationID)
				_ = d.Ack(false)
				return
			}
		}
	}

	// Idempotency layer 2: PostgreSQL lookup by investigation_id. The unique
	// index on investigations.investigation_id (migration 001) is the
	// authoritative guard, but a pre-check lets us ack-and-skip cleanly
	// without producing duplicate-key spam in the logs.
	if msg.InvestigationID != "" {
		existing, err := w.alertInvestigationStore.GetAlertInvestigation(ctx, msg.InvestigationID)
		if err != nil {
			logger.Warn("Idempotency lookup failed; continuing", "component", "investigate-worker", "investigation_id", msg.InvestigationID, "error", err)
		} else if existing != nil {
			logger.Info("Investigation already exists; acking duplicate", "component", "investigate-worker", "investigation_id", msg.InvestigationID, "status", existing.Status)
			_ = d.Ack(false)
			return
		}
	}

	var mmPrimary string
	var slackCh, slackTS string
	var alertThreadIDs []struct {
		threadID, channelID, provider string
	}
	for _, alert := range msg.Alerts {
		alertRecord, err := w.alertStore.GetOpenByFingerprint(alert.Fingerprint)
		if err != nil {
			logger.Warn("Failed to look up alert", "component", "investigate-worker", "fingerprint", alert.Fingerprint, "error", err)
			continue
		}
		if alertRecord == nil {
			continue
		}
		for _, dt := range alertRecord.DeliveryTargets {
			threadID := dt.PostID
			channelID := dt.Channel
			prov := strings.ToLower(strings.TrimSpace(dt.Provider))
			if threadID == "" {
				continue
			}
			alertThreadIDs = append(alertThreadIDs, struct{ threadID, channelID, provider string }{threadID, channelID, prov})
			if prov == "slack" && slackTS == "" {
				slackCh, slackTS = channelID, threadID
			} else if prov == "mattermost" && mmPrimary == "" {
				mmPrimary = threadID
			}
		}
	}
	primaryThreadIDForRecord := mmPrimary
	if primaryThreadIDForRecord == "" {
		primaryThreadIDForRecord = slackTS
	}
	var triageResultID *uuid.UUID
	if msg.TriageResultID != "" {
		if parsed, err := uuid.Parse(msg.TriageResultID); err == nil {
			triageResultID = &parsed
		}
	}

	record := store.AlertInvestigationRecord{
		AlertInvestigationID:    msg.InvestigationID,
		Alerts:                  msg.Alerts,
		CorrelationKey:          msg.CorrelationKey,
		Status:                  "pending",
		PrimaryThreadID:         primaryThreadIDForRecord,
		SlackChannelID:          slackCh,
		SlackThreadTS:           slackTS,
		MMThreadID:              mmPrimary,
		PrimaryAlertFingerprint: msg.PrimaryAlertFingerprint,
		PrimaryAlertNumber:      msg.PrimaryAlertNumber,
		TriageResultID:          triageResultID,
		TriageDecision:          msg.TriageDecision,
		TriageEnrichment:        triageEnrichmentToMap(msg.TriageEnrichment),
	}
	createStart := time.Now()
	created, err := w.alertInvestigationStore.CreateAlertInvestigation(ctx, record)
	if err != nil {
		if store.IsDuplicateKey(err) {
			logger.Info("Duplicate investigation insert; acking", "component", "investigate-worker", "investigation_id", msg.InvestigationID)
			_ = d.Ack(false)
			return
		}
		logger.Error("Failed to create investigation record", "component", "investigate-worker", "error", err)
		w.scheduleRetryOrDeadLetter(ctx, msg, d, "create_investigation", err)
		return
	}
	investigationID := created.AlertInvestigationID
	metrics.InvestigateWorkerCreateLatencyMs.Set(time.Since(createStart).Milliseconds())
	logger.Info("Created pending investigation", "component", "investigate-worker", "investigation_id", investigationID)

	w.publishSSE("investigation_created", map[string]any{
		"alert_investigation_id": investigationID,
		"status":                 "pending",
	})

	if mmPrimary != "" || slackTS != "" {
		logger.Info("Investigation threads resolved", "component", "investigate-worker", "investigation_id", investigationID, "mattermost_thread", mmPrimary, "slack_channel", slackCh, "slack_thread", slackTS)

		if mmPrimary != "" && slackTS != "" && w.mmClient != nil && w.mmClient.Enabled() && w.slackClient != nil && w.slackClient.Enabled() {
			mmNote := "This investigation is also active in **Slack** (same correlation window)."
			if _, err := w.mmClient.ReplyToPost(ctx, mmPrimary, mmNote, nil); err != nil {
				logger.Warn("Failed to post cross-provider note to Mattermost", "component", "investigate-worker", "error", err)
			}
			slNote := "This investigation is also active in **Mattermost**."
			if _, err := w.slackClient.PostThreadReply(ctx, slackCh, slackTS, slNote); err != nil {
				logger.Warn("Failed to post cross-provider note to Slack", "component", "investigate-worker", "error", err)
			}
		}

		var slackPermalink string
		if slackTS != "" && w.slackClient != nil && w.slackClient.Enabled() {
			if pl, err := w.slackClient.GetPermalink(ctx, slackCh, slackTS); err != nil {
				logger.Warn("Failed to get Slack permalink for primary thread", "component", "investigate-worker", "error", err)
			} else {
				slackPermalink = pl
			}
		}

		linkMsg := fmt.Sprintf("🔗 **Related investigation**: %s\n\nAn investigation is in progress. See the other alert thread(s) for updates.", investigationID)
		for _, ti := range alertThreadIDs {
			if ti.provider == "mattermost" && ti.threadID == mmPrimary {
				continue
			}
			if ti.provider == "slack" && ti.channelID == slackCh && ti.threadID == slackTS {
				continue
			}
			if ti.provider == "slack" && w.slackClient != nil && w.slackClient.Enabled() {
				msg := linkMsg
				if slackPermalink != "" {
					msg = fmt.Sprintf("🔗 **Related investigation**: %s\nMain thread: <%s|View primary thread>\n\nAn investigation is in progress. Follow the main thread for updates.", investigationID, slackPermalink)
				}
				if _, err := w.slackClient.PostThreadReply(ctx, ti.channelID, ti.threadID, msg); err != nil {
					logger.Warn("Failed to post investigation link to Slack thread", "component", "investigate-worker", "thread_id", ti.threadID, "error", err)
				}
				continue
			}
			if w.mmClient != nil && w.mmClient.Enabled() {
				if _, err := w.mmClient.ReplyToPost(ctx, ti.threadID, linkMsg, nil); err != nil {
					logger.Warn("Failed to post investigation link to thread", "component", "investigate-worker", "thread_id", ti.threadID, "error", err)
				}
			}
		}
	} else if w.mmClient != nil && w.mmClient.Enabled() && w.cfg.InvestigationChannel != "" {
		channelID, err := w.mmClient.GetChannelByName(ctx, w.cfg.InvestigationChannel)
		if err != nil {
			logger.Error("Failed to resolve investigation channel", "component", "investigate-worker", "error", err)
		} else if _, postErr := w.mmClient.CreatePost(ctx, channelID, fmt.Sprintf("**%s** — %d alert(s)", investigationID, len(msg.Alerts)), nil); postErr != nil {
			logger.Warn("Failed to post investigation notice to mattermost", "component", "investigate-worker", "investigation_id", investigationID, "error", postErr)
		}
	}

	if w.notifier != nil {
		w.notifier.NotifyPending()
	}
	_ = d.Ack(false)
}

// scheduleRetryOrDeadLetter routes a failed delivery either to the next
// retry queue (with exponential backoff via TTL DLX in topology.go) or, if
// retries are exhausted / no publisher is wired, dead-letters via Nack so
// it ends up on the alga.dlq exchange.
func (w *InvestigateWorker) scheduleRetryOrDeadLetter(ctx context.Context, msg rabbitmq.InvestigateMessage, d amqp.Delivery, stage string, cause error) {
	dk := dedupeKey(msg)
	var retryFn func() error
	if w.publisher != nil {
		retryFn = func() error {
			msg.RetryCount++
			return w.publisher.PublishInvestigationRetry(ctx, msg)
		}
	}
	retryOrDeadLetter(ctx, w.vkClient, dk, retryFn, d, "Investigation", msg.InvestigationID, stage, cause)
}

func (w *InvestigateWorker) publishSSE(eventType string, data any) {
	if w.ssePublisher == nil {
		return
	}
	w.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

func triageEnrichmentToMap(e rabbitmq.TriageEnrichment) map[string]any {
	out := map[string]any{}
	if e.ServiceOwner != "" {
		out["service_owner"] = e.ServiceOwner
	}
	if e.RunbookURL != "" {
		out["runbook_url"] = e.RunbookURL
	}
	if e.PastRootCause != "" {
		out["past_root_cause"] = e.PastRootCause
	}
	if e.PastResolution != "" {
		out["past_resolution"] = e.PastResolution
	}
	if len(e.SuggestedActions) > 0 {
		out["suggested_actions"] = e.SuggestedActions
	}
	if len(e.SimilarInvestigationIDs) > 0 {
		out["similar_investigation_ids"] = e.SimilarInvestigationIDs
	}
	if len(e.Custom) > 0 {
		out["custom"] = e.Custom
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitMessage(msg string, maxLen int) []string {
	if len(msg) <= maxLen {
		return []string{msg}
	}

	var parts []string
	for len(msg) > maxLen {
		idx := strings.LastIndex(msg[:maxLen], "\n")
		if idx == -1 {
			idx = maxLen
		}
		parts = append(parts, msg[:idx])
		msg = msg[idx:]
	}
	if len(msg) > 0 {
		parts = append(parts, msg)
	}
	return parts
}
