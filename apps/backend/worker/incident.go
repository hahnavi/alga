package worker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"alga/cancellation"
	"alga/ics"
	"alga/incident"
	"alga/logger"
	"alga/metrics"
	"alga/oncall"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

const incidentDedupeKeyTTL = 24 * time.Hour

type IncidentWorker struct {
	incidentStore              store.IncidentStore
	incidentInvestigationStore store.IncidentInvestigationStore
	threadStore                store.InvestigationThreadStore
	alertStore                 store.Store
	publisher                  *rabbitmq.Publisher
	ssePublisher               *sse.DualPublisher
	vkClient                   *valkey.Client
	cancelSet                  *valkey.CancelSet
	icsRoleStore               store.ICSRoleStore
	icsPublisher               *rabbitmq.Publisher
	serviceStore               store.ServiceStore
	onCallResolver             *oncall.Resolver
	onCallStore                store.OnCallStore
	escalationStore            store.EscalationStore
	userStore                  store.UserStore
	escalationPublisher        *rabbitmq.Publisher
	notifier                   InvestigationNotifier
}

func NewIncidentWorker(
	incidentStore store.IncidentStore,
	incidentInvestigationStore store.IncidentInvestigationStore,
	alertStore store.Store,
	publisher *rabbitmq.Publisher,
	ssePublisher *sse.DualPublisher,
	vkClient *valkey.Client,
	icsRoleStore store.ICSRoleStore,
	icsPublisher *rabbitmq.Publisher,
	serviceStore store.ServiceStore,
	onCallResolver *oncall.Resolver,
	onCallStore store.OnCallStore,
	escalationStore store.EscalationStore,
	userStore store.UserStore,
	escalationPublisher *rabbitmq.Publisher,
) *IncidentWorker {
	return &IncidentWorker{
		incidentStore:              incidentStore,
		incidentInvestigationStore: incidentInvestigationStore,
		alertStore:                 alertStore,
		publisher:                  publisher,
		ssePublisher:               ssePublisher,
		vkClient:                   vkClient,
		icsRoleStore:               icsRoleStore,
		icsPublisher:               icsPublisher,
		serviceStore:               serviceStore,
		onCallResolver:             onCallResolver,
		onCallStore:                onCallStore,
		escalationStore:            escalationStore,
		userStore:                  userStore,
		escalationPublisher:        escalationPublisher,
	}
}

func (w *IncidentWorker) SetNotifier(n InvestigationNotifier) {
	w.notifier = n
}

func (w *IncidentWorker) SetThreadStore(ts store.InvestigationThreadStore) {
	w.threadStore = ts
}

func (w *IncidentWorker) Queue() string {
	return rabbitmq.QueueIncidentProcess
}

func (w *IncidentWorker) SetCancelSet(cs *valkey.CancelSet) { w.cancelSet = cs }

func (w *IncidentWorker) PrefetchCount() int {
	return 1
}

func (w *IncidentWorker) Handle(ctx context.Context, d amqp.Delivery) {
	msg, err := rabbitmq.UnmarshalIncidentMessage(d)
	if err != nil {
		logger.Error("Failed to unmarshal incident message", "component", "incident-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	traceID := msg.TraceID
	if traceID == "" {
		traceID = msg.CorrelationKey
	}
	if msg.InvestigationID != "" && cancellation.InvestigationCancelled(ctx, w.cancelSet, msg.InvestigationID) {
		logger.Info("Dropping incident job; source investigation deleted", "component", "incident-worker", "investigation_id", msg.InvestigationID)
		_ = d.Ack(false)
		return
	}
	logger.Info("Processing incident message", "component", "incident-worker", "alert_count", len(msg.Alerts), "severity", msg.Severity, "trace_id", traceID, "retry", msg.RetryCount)

	if w.vkClient != nil && msg.DedupeKey != "" {
		dk := "alga:incident-dedupe:" + msg.DedupeKey
		ok, err := w.vkClient.SetNX(ctx, dk, "1", incidentDedupeKeyTTL)
		if err != nil {
			logger.Warn("Incident dedupe SETNX failed; continuing", "component", "incident-worker", "error", err)
		} else if !ok {
			logger.Info("Dropping duplicate incident delivery", "component", "incident-worker", "dedupe_key", msg.DedupeKey)
			_ = d.Ack(false)
			return
		}
	}

	incidentNumber, err := w.incidentStore.ReserveIncidentNumber(ctx)
	if err != nil {
		logger.Error("Failed to reserve incident number", "component", "incident-worker", "error", err)
		w.scheduleRetryOrDeadLetter(ctx, msg, d, "reserve_incident_number", err)
		return
	}

	title := fmt.Sprintf("Incident %d", incidentNumber)
	if len(msg.Alerts) > 0 {
		if name, ok := msg.Alerts[0].Labels["alertname"]; ok && name != "" {
			title = name
		}
	}

	now := time.Now().UTC()
	severity := msg.Severity
	if severity == "" {
		severity = "warning"
	}
	impact := msg.ImpactLevel
	if impact == "" {
		impact = "medium"
	}
	priority := incident.ComputePriority(severity, impact)
	respondDuration, resolveDuration := PriorityToSLATargets(priority)
	slaRespondAt := now.Add(respondDuration)
	slaResolveAt := now.Add(resolveDuration)
	record := &store.IncidentRecord{
		IncidentNumber:     incidentNumber,
		Title:              title,
		Severity:           severity,
		ImpactLevel:        impact,
		Priority:           priority,
		IncidentType:       "alert",
		Status:             "detected",
		ServiceID:          msg.ServiceID,
		StartedAt:          &now,
		SLATargetRespondAt: &slaRespondAt,
		SLATargetResolveAt: &slaResolveAt,
	}

	created, err := w.incidentStore.CreateIncident(ctx, record)
	if err != nil {
		logger.Error("Failed to create incident", "component", "incident-worker", "incident_number", incidentNumber, "error", err)
		w.scheduleRetryOrDeadLetter(ctx, msg, d, "create_incident", err)
		return
	}

	incidentID := strconv.FormatInt(created.IncidentNumber, 10)

	for _, alert := range msg.Alerts {
		if alert.Fingerprint == "" {
			continue
		}
		if err := w.alertStore.LinkAlertToIncident(ctx, alert.Fingerprint, created.IncidentNumber); err != nil {
			logger.Warn("Failed to link alert to incident", "component", "incident-worker", "fingerprint", alert.Fingerprint, "incident_number", created.IncidentNumber, "error", err)
		}
	}

	if msg.InvestigationID != "" {
		active, err := w.incidentInvestigationStore.GetActiveIncidentInvestigationByIncident(ctx, created.IncidentNumber)
		if err == nil && active == nil {
			_, createErr := w.incidentInvestigationStore.CreateIncidentInvestigation(ctx, store.IncidentInvestigationRecord{
				IncidentNumber:             created.IncidentNumber,
				SourceAlertInvestigationID: nil,
				Status:                     "pending",
			})
			if createErr != nil {
				logger.Warn("Failed to create incident investigation for linked alert investigation", "component", "incident-worker", "incident_number", created.IncidentNumber, "error", createErr)
			}
		}
	}

	if err := w.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: created.IncidentNumber,
		EventType:      "created",
		ActorType:      "system",
		Message:        fmt.Sprintf("Incident %s created from correlation key %s", incidentID, msg.CorrelationKey),
	}); err != nil {
		logger.Warn("Failed to add timeline entry for incident", "component", "incident-worker", "incident_number", created.IncidentNumber, "error", err)
	}

	if err := w.incidentStore.TransitionIncidentStatus(ctx, created.IncidentNumber, []string{"detected"}, "active"); err != nil {
		logger.Warn("Failed to transition incident from detected to active", "component", "incident-worker", "incident_number", created.IncidentNumber, "error", err)
	} else {
		if err := w.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
			IncidentNumber: created.IncidentNumber,
			EventType:      "acknowledged",
			ActorType:      "system",
			Message:        "Incident auto-acknowledged",
		}); err != nil {
			logger.Warn("Failed to add incident auto-ack timeline entry", "component", "incident-worker", "incident_number", created.IncidentNumber, "error", err)
		}
	}

	if w.ssePublisher != nil {
		w.ssePublisher.Publish(sse.Event{
			Type: "incident_created",
			Data: map[string]any{
				"incident_number": created.IncidentNumber,
				"severity":        created.Severity,
				"status":          created.Status,
			},
		})
	}

	metrics.IncidentsCreatedTotal.Add(1)
	// Mirror the API create path so the active gauge counts correlation-created
	// incidents too (resolve/cancel decrement it for every creation path).
	metrics.IncidentsActive.Add(1)
	logger.Info("Created incident", "component", "incident-worker", "incident_number", created.IncidentNumber, "severity", created.Severity)

	w.ensureIncidentInvestigation(ctx, created)

	if w.icsRoleStore != nil && w.icsPublisher != nil {
		w.autoAssignIC(ctx, created)
		icsMsg := rabbitmq.ICSProvisionMessage{
			IncidentNumber: created.IncidentNumber,
			TraceID:        traceID,
		}
		if err := w.icsPublisher.PublishICSProvision(ctx, icsMsg); err != nil {
			logger.Warn("Failed to publish ICS provision message", "component", "incident-worker", "incident_number", created.IncidentNumber, "error", err)
		}
	}

	_ = d.Ack(false)
}

func (w *IncidentWorker) ensureIncidentInvestigation(ctx context.Context, inc *store.IncidentRecord) {
	if w.incidentInvestigationStore == nil || w.incidentStore == nil || inc == nil {
		return
	}
	invs, err := w.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, inc.IncidentNumber)
	if err != nil {
		logger.Warn("Failed to check incident investigations", "component", "incident-worker", "incident_number", inc.IncidentNumber, "error", err)
		return
	}
	for _, inv := range invs {
		if !store.IsTerminalInvestigationStatus(inv.Status) {
			return
		}
	}

	inv, err := w.incidentInvestigationStore.CreateIncidentInvestigation(ctx, store.IncidentInvestigationRecord{
		IncidentNumber: inc.IncidentNumber,
		Status:         "pending",
	})
	if err != nil {
		logger.Error("Failed to create incident investigation", "component", "incident-worker", "incident_number", inc.IncidentNumber, "error", err)
		return
	}
	if inv == nil {
		return
	}
	if w.ssePublisher != nil {
		w.ssePublisher.Publish(sse.Event{
			Type: "investigation_created",
			Data: map[string]any{
				"investigation_id": inv.IncidentInvestigationID,
				"incident_number":  inc.IncidentNumber,
				"status":           inv.Status,
			},
		})
		w.ssePublisher.Publish(sse.Event{
			Type: "incident_updated",
			Data: map[string]string{"incident_number": strconv.FormatInt(inc.IncidentNumber, 10)},
		})
	}
	if w.notifier != nil {
		w.notifier.NotifyPending()
	}
	logger.Info("Created incident investigation", "component", "incident-worker", "incident_number", inc.IncidentNumber, "investigation_id", inv.IncidentInvestigationID)

	if w.threadStore != nil {
		if _, threadErr := w.threadStore.EnsureThread(ctx, store.ThreadOwnerIncidentInvestigation, strconv.FormatInt(inc.IncidentNumber, 10)); threadErr != nil {
			logger.Warn("Failed to ensure incident thread", "component", "incident-worker", "incident_number", inc.IncidentNumber, "error", threadErr)
		}
	}
}

func (w *IncidentWorker) scheduleRetryOrDeadLetter(ctx context.Context, msg rabbitmq.IncidentMessage, d amqp.Delivery, stage string, cause error) {
	var dk string
	if msg.DedupeKey != "" {
		dk = "alga:incident-dedupe:" + msg.DedupeKey
	}
	var retryFn func() error
	if w.publisher != nil {
		retryFn = func() error {
			msg.RetryCount++
			return w.publisher.PublishIncidentRetry(ctx, msg)
		}
	}
	retryOrDeadLetter(ctx, w.vkClient, dk, retryFn, d, "Incident", msg.DedupeKey, stage, cause)
}

func (w *IncidentWorker) autoAssignIC(ctx context.Context, inc *store.IncidentRecord) {
	onCallUserID, err := oncall.ResolveOnCallUserForIncident(ctx, inc, w.serviceStore, w.escalationStore, w.onCallStore, w.onCallResolver)
	if err != nil {
		logger.Warn("Failed to resolve on-call user for IC assignment", "component", "incident-worker", "incident_number", inc.IncidentNumber, "error", err)
		return
	}
	if onCallUserID == nil {
		logger.Info("No on-call user found for IC assignment", "component", "incident-worker", "incident_number", inc.IncidentNumber)
		return
	}

	svc, _ := w.serviceStore.GetService(ctx, inc.ServiceID.String())
	policyID := uuid.Nil
	if svc != nil && svc.EscalationPolicyID != nil {
		policyID = *svc.EscalationPolicyID
	}
	_, err = w.icsRoleStore.AssignRole(ctx, inc.IncidentNumber, ics.RoleIncidentCommander, *onCallUserID, nil, nil)
	if err != nil {
		logger.Warn("Failed to auto-assign IC", "component", "incident-worker", "incident_number", inc.IncidentNumber, "user_id", onCallUserID, "error", err)
		return
	}
	if err := w.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: inc.IncidentNumber,
		EventType:      "ics_role_assigned",
		ActorType:      "system",
		Message:        "Auto-assigned Incident Commander (on-call)",
	}); err != nil {
		logger.Warn("Failed to add IC-assignment timeline entry", "component", "incident-worker", "incident_number", inc.IncidentNumber, "error", err)
	}
	logger.Info("Auto-assigned IC from on-call", "component", "incident-worker", "incident_number", inc.IncidentNumber, "user_id", onCallUserID)

	if w.escalationPublisher != nil && policyID != uuid.Nil {
		verdict := claimEscalationFirstPublish(ctx, w.vkClient, inc.IncidentNumber, policyID.String(), 1)
		if verdict != guardAllow {
			logger.Info("Initial escalation suppressed by guard", "component", "incident-worker", "incident_number", inc.IncidentNumber, "reason", verdict.String())
			return
		}
		escMsg := rabbitmq.EscalationMessage{
			IncidentNumber: inc.IncidentNumber,
			PolicyID:       policyID,
			Level:          1,
			MaxRetries:     rabbitmq.MaxEscalationRetries,
		}
		if pubErr := w.escalationPublisher.PublishEscalation(ctx, escMsg); pubErr != nil {
			logger.Warn("Failed to publish initial escalation", "component", "incident-worker", "incident_number", inc.IncidentNumber, "error", pubErr)
		} else {
			logger.Info("Published initial escalation for incident", "component", "incident-worker", "incident_number", inc.IncidentNumber)
		}
	}
}
