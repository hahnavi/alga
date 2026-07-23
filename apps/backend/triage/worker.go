package triage

import (
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/cancellation"
	"alga/logger"
	"alga/rabbitmq"
	"alga/store"
	"alga/valkey"
)

type Worker struct {
	engine     *Engine
	publisher  *rabbitmq.Publisher
	alertStore store.Store
	auditStore store.AuditStore
	cancelSet  *valkey.CancelSet
}

func NewWorker(engine *Engine, publisher *rabbitmq.Publisher, alertStore store.Store) *Worker {
	return &Worker{engine: engine, publisher: publisher, alertStore: alertStore}
}

func (w *Worker) SetCancelSet(cs *valkey.CancelSet) { w.cancelSet = cs }
func (w *Worker) SetAuditStore(s store.AuditStore)  { w.auditStore = s }

func (w *Worker) Queue() string {
	return rabbitmq.QueueTriageProcess
}

func (w *Worker) PrefetchCount() int {
	return 5
}

func (w *Worker) Handle(ctx context.Context, d amqp.Delivery) {
	var msg rabbitmq.TriageMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.ErrorCtx(ctx, "Failed to unmarshal triage message", "component", "triage", "error", err)
		_ = d.Nack(false, false)
		return
	}

	primary := ""
	primaryNum := int64(0)
	if len(msg.Alerts) > 0 {
		primary = msg.Alerts[0].Fingerprint
		primaryNum = msg.Alerts[0].AlertNumber
	}
	if cancellation.AlertCancelled(ctx, w.cancelSet, w.alertStore, primary, primaryNum) {
		logger.ErrorCtx(ctx, "Dropping triage; primary alert deleted", "component", "triage", "fingerprint", primary)
		_ = d.Ack(false)
		return
	}

	result, err := w.engine.Process(ctx, msg)
	if err != nil {
		logger.ErrorCtx(ctx, "Triage processing failed", "component", "triage", "correlation_key", msg.CorrelationKey, "error", err)
		w.handleRetry(ctx, d, msg, err)
		return
	}

	var handleErr error
	switch result.Response.Decision {
	case store.TriageDecisionAutoResolve:
		handleErr = w.handleAutoResolve(ctx, msg, result)
	case store.TriageDecisionSuppress:
		handleErr = w.handleSuppress(ctx, msg, result)
	case store.TriageDecisionEscalate:
		handleErr = w.handleEscalate(ctx, msg, result)
	case store.TriageDecisionEnrichOnly:
		fallthrough
	default:
		handleErr = w.handleInvestigate(ctx, msg, result)
	}
	if handleErr != nil {
		w.handleRetry(ctx, d, msg, handleErr)
		return
	}
	_ = d.Ack(false)
}

func (w *Worker) handleInvestigate(ctx context.Context, msg rabbitmq.TriageMessage, result *TriageResultWrapper) error {
	invMsg := rabbitmq.InvestigateMessage{
		InvestigationID:  msg.InvestigationID,
		Alerts:           msg.Alerts,
		Severity:         msg.Severity,
		CorrelationKey:   msg.CorrelationKey,
		DedupeKey:        msg.DedupeKey,
		TriageResultID:   result.Record.ID.String(),
		TriageDecision:   result.Response.Decision,
		TriageSeverity:   result.Response.Severity,
		TriageCategory:   result.Response.Category,
		TriageReasoning:  result.Response.Reasoning,
		TriageConfidence: result.Response.Confidence,
	}
	if result.Response.Enrichment.ServiceOwner != "" || result.Response.Enrichment.RunbookURL != "" {
		enrich := rabbitmq.TriageEnrichment{
			ServiceOwner:   result.Response.Enrichment.ServiceOwner,
			RunbookURL:     result.Response.Enrichment.RunbookURL,
			PastRootCause:  result.Response.Enrichment.PastRootCause,
			PastResolution: result.Response.Enrichment.PastResolution,
		}
		if result.Response.SuggestedActions != nil {
			enrich.SuggestedActions = result.Response.SuggestedActions
		}
		invMsg.TriageEnrichment = enrich
	}
	if err := w.publisher.PublishInvestigation(ctx, invMsg); err != nil {
		logger.ErrorCtx(ctx, "Failed to publish investigate message", "component", "triage", "correlation_key", msg.CorrelationKey, "error", err)
		return err
	}
	return nil
}

func (w *Worker) handleAutoResolve(ctx context.Context, msg rabbitmq.TriageMessage, result *TriageResultWrapper) error {
	logger.InfoCtx(ctx, "Auto-resolving alert group", "component", "triage", "correlation_key", msg.CorrelationKey)
	if w.alertStore == nil {
		logger.Warn("Cannot auto-resolve: no alert store configured")
		return nil
	}
	now := time.Now().UTC()
	ev := store.AlertEvent{Type: "resolved", Timestamp: now, Source: "triage_auto_resolve"}
	for _, ca := range msg.Alerts {
		if err := w.alertStore.UpdateStatus(ca.Fingerprint, "resolved", &ev); err != nil {
			logger.ErrorCtx(ctx, "Failed to auto-resolve alert", "component", "triage", "fingerprint", ca.Fingerprint, "error", err)
			return err
		}
	}
	return nil
}

func (w *Worker) handleSuppress(ctx context.Context, msg rabbitmq.TriageMessage, result *TriageResultWrapper) error {
	logger.InfoCtx(ctx, "Suppressing alert group for correlation key", "component", "triage", "correlation_key", msg.CorrelationKey)
	if w.alertStore == nil {
		logger.Warn("Cannot suppress: no alert store configured")
		return nil
	}
	for _, ca := range msg.Alerts {
		if err := w.alertStore.UpdateStatusSilenced(ca.Fingerprint); err != nil {
			logger.ErrorCtx(ctx, "Failed to suppress alert", "component", "triage", "fingerprint", ca.Fingerprint, "error", err)
			return err
		}
		// Audit each suppression so the transition is attributable. The
		// auto_resolve path emits a persisted alert event via UpdateStatus;
		// UpdateStatusSilenced has no event parameter, so we audit here only.
		if w.auditStore != nil {
			w.auditStore.Log(store.AuditAlertSuppressed, nil, "triage:engine", "", "", true, map[string]any{
				"fingerprint":      ca.Fingerprint,
				"correlation_key":  msg.CorrelationKey,
				"triage_result_id": result.Record.ID.String(),
				"reasoning":        result.Response.Reasoning,
			})
		}
	}
	return nil
}

func (w *Worker) handleEscalate(ctx context.Context, msg rabbitmq.TriageMessage, result *TriageResultWrapper) error {
	logger.InfoCtx(ctx, "Escalating alert group for correlation key", "component", "triage", "correlation_key", msg.CorrelationKey)
	return w.handleInvestigate(ctx, msg, result)
}

func (w *Worker) handleRetry(ctx context.Context, d amqp.Delivery, msg rabbitmq.TriageMessage, processErr error) {
	if w.publisher == nil {
		_ = d.Nack(false, true)
		return
	}
	msg.RetryCount++
	if msg.RetryCount > rabbitmq.MaxTriageRetries {
		_ = d.Nack(false, false)
		logger.WarnCtx(ctx, "Triage retries exhausted, falling back to investigate", "component", "triage", "correlation_key", msg.CorrelationKey)
		w.fallbackToInvestigate(ctx, msg)
		return
	}
	retryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := w.publisher.PublishTriageRetry(retryCtx, msg); err != nil {
		_ = d.Nack(false, true)
		return
	}
	_ = d.Ack(false)
}

func (w *Worker) fallbackToInvestigate(ctx context.Context, msg rabbitmq.TriageMessage) {
	invMsg := rabbitmq.InvestigateMessage{
		InvestigationID: msg.InvestigationID,
		Alerts:          msg.Alerts,
		Severity:        msg.Severity,
		CorrelationKey:  msg.CorrelationKey,
		DedupeKey:       msg.DedupeKey,
	}
	if err := w.publisher.PublishInvestigation(ctx, invMsg); err != nil {
		logger.ErrorCtx(ctx, "Failed to publish fallback investigate message", "component", "triage", "correlation_key", msg.CorrelationKey, "error", err)
	}
}
