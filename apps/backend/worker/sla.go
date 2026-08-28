package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/logger"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/valkey"
	"alga/webhook"
)

type SLAWorker struct {
	incidentStore        store.IncidentStore
	ssePublisher         *sse.DualPublisher
	vkClient             *valkey.Client
	coordinationStore    store.IncidentCoordinationStore
	icsRoleStore         store.ICSRoleStore
	forwarder            webhook.InvestigationAgentForwarder
	publisher            *rabbitmq.Publisher
	serviceStore         store.ServiceStore
	statusUpdateInterval time.Duration
}

func NewSLAWorker(incidentStore store.IncidentStore, ssePublisher *sse.DualPublisher, vkClient *valkey.Client) *SLAWorker {
	return &SLAWorker{
		incidentStore: incidentStore,
		ssePublisher:  ssePublisher,
		vkClient:      vkClient,
	}
}

func (w *SLAWorker) SetCoordinationStore(s store.IncidentCoordinationStore) { w.coordinationStore = s }
func (w *SLAWorker) SetICSRoleStore(s store.ICSRoleStore)                   { w.icsRoleStore = s }
func (w *SLAWorker) SetForwarder(f webhook.InvestigationAgentForwarder)     { w.forwarder = f }
func (w *SLAWorker) SetEscalationPublisher(p *rabbitmq.Publisher)           { w.publisher = p }
func (w *SLAWorker) SetServiceStore(s store.ServiceStore)                   { w.serviceStore = s }
func (w *SLAWorker) SetStatusUpdateInterval(d time.Duration)                { w.statusUpdateInterval = d }

func (w *SLAWorker) Queue() string {
	return rabbitmq.QueueSLASweep
}

func (w *SLAWorker) PrefetchCount() int {
	return 1
}

func (w *SLAWorker) Handle(ctx context.Context, d amqp.Delivery) {
	var msg rabbitmq.SLASweepMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error("Failed to unmarshal SLA sweep message", "component", "sla-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	logger.Info("Processing SLA sweep tick", "component", "sla-worker")

	incidents, err := w.incidentStore.ListSLAEligibleIncidents(ctx)
	if err != nil {
		logger.Error("Failed to list SLA-eligible incidents", "component", "sla-worker", "error", err)
		_ = d.Nack(false, false)
		return
	}

	now := time.Now().UTC()
	breached := 0

	for _, inc := range incidents {
		incidentID := strconv.FormatInt(inc.IncidentNumber, 10)
		if inc.SLATargetRespondAt != nil && inc.SLAAcknowledgedAt == nil && now.After(*inc.SLATargetRespondAt) {
			if w.markBreachDeduped(ctx, incidentID, "response") {
				if err := w.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
					IncidentNumber: inc.IncidentNumber,
					EventType:      "sla_breach",
					ActorType:      "system",
					Message:        "Response SLA breached",
				}); err != nil {
					logger.Error("Failed to add response SLA breach timeline entry for incident", "component", "sla-worker", "incident_id", incidentID, "error", err)
				}
				metrics.SLABreachesResponse.Add(1)
				w.publishSLABreach(inc, "response")
				w.triggerEscalationOnBreach(ctx, inc)
				breached++
			}
		}

		if inc.SLATargetResolveAt != nil && inc.SLAResolvedAt == nil && now.After(*inc.SLATargetResolveAt) {
			if w.markBreachDeduped(ctx, incidentID, "resolve") {
				if err := w.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
					IncidentNumber: inc.IncidentNumber,
					EventType:      "sla_breach",
					ActorType:      "system",
					Message:        "Resolve SLA breached",
				}); err != nil {
					logger.Error("Failed to add resolve SLA breach timeline entry for incident", "component", "sla-worker", "incident_id", incidentID, "error", err)
				}
				metrics.SLABreachesResolve.Add(1)
				w.publishSLABreach(inc, "resolve")
				breached++
			}
		}

		w.sweepCommsStaleness(ctx, inc)
	}

	logger.Info("SLA sweep complete", "component", "sla-worker", "incidents_checked", len(incidents), "breaches", breached)
	_ = d.Ack(false)
}

func (w *SLAWorker) markBreachDeduped(ctx context.Context, incidentID, breachType string) bool {
	if w.vkClient == nil {
		return true
	}
	key := breachDedupKey(incidentID, breachType)
	ok, err := w.vkClient.SetNX(ctx, key, "1", 24*time.Hour)
	if err != nil {
		logger.Warn("SLA breach dedup SETNX failed", "component", "sla-worker", "key", key, "error", err)
		return true
	}
	return ok
}

func breachDedupKey(incidentID, breachType string) string {
	return fmt.Sprintf("alga:sla:breach:%s:%s", incidentID, breachType)
}

// ClearBreachDedupKeys deletes the per-incident SLA breach dedup markers so a
// reopened incident can re-breach within the marker TTL. Part of the
// reopen reset; safe to call with a nil client (no Valkey, no dedup either).
func ClearBreachDedupKeys(ctx context.Context, vk *valkey.Client, incidentNumber int64) {
	if vk == nil {
		return
	}
	id := strconv.FormatInt(incidentNumber, 10)
	if err := vk.Del(ctx, breachDedupKey(id, "response"), breachDedupKey(id, "resolve")); err != nil {
		logger.Warn("failed to clear SLA breach dedup keys", "component", "sla-worker", "incident_id", id, "error", err)
	}
}

func (w *SLAWorker) publishSLABreach(inc store.IncidentRecord, breachType string) {
	if w.ssePublisher == nil {
		return
	}

	w.ssePublisher.Publish(sse.Event{
		Type: "incident_sla_breach",
		Data: map[string]any{
			"incident_number": inc.IncidentNumber,
			"breach_type":     breachType,
		},
	})

	if inc.CommanderID != nil {
		w.ssePublisher.PublishToUser(inc.CommanderID.String(), sse.Event{
			Type: "incident_sla_breach",
			Data: map[string]any{
				"incident_number": inc.IncidentNumber,
				"breach_type":     breachType,
			},
		})
	}
}

func (w *SLAWorker) triggerEscalationOnBreach(ctx context.Context, inc store.IncidentRecord) {
	if w.publisher == nil {
		return
	}
	policyID := inc.EscalationPolicyID
	if policyID == nil && inc.ServiceID != nil && w.serviceStore != nil {
		if svc, err := w.serviceStore.GetService(ctx, inc.ServiceID.String()); err == nil && svc != nil {
			policyID = svc.EscalationPolicyID
		}
	}
	if policyID == nil {
		return
	}
	verdict := claimEscalationFirstPublish(ctx, w.vkClient, inc.IncidentNumber, policyID.String(), 1)
	if verdict != guardAllow {
		logger.Info("SLA escalation suppressed by guard", "component", "sla-worker", "incident_number", inc.IncidentNumber, "reason", verdict.String())
		return
	}
	if err := w.publisher.PublishEscalation(ctx, rabbitmq.EscalationMessage{
		IncidentNumber: inc.IncidentNumber,
		PolicyID:       *policyID,
		Level:          1,
		MaxRetries:     rabbitmq.MaxEscalationRetries,
	}); err != nil {
		logger.Warn("Failed to publish SLA-triggered escalation", "component", "sla-worker", "incident_number", inc.IncidentNumber, "error", err)
		return
	}
	if err := w.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: inc.IncidentNumber,
		EventType:      "sla_escalation",
		ActorType:      "system",
		Message:        "Response SLA breach triggered escalation",
	}); err != nil {
		logger.Error("Failed to add SLA escalation timeline entry", "component", "sla-worker", "incident_number", inc.IncidentNumber, "error", err)
	}
}

func PriorityToSLATargets(priority string) (respond time.Duration, resolve time.Duration) {
	switch priority {
	case "P1":
		return 15 * time.Minute, 4 * time.Hour
	case "P2":
		return 30 * time.Minute, 8 * time.Hour
	case "P3":
		return 2 * time.Hour, 24 * time.Hour
	case "P4":
		return 8 * time.Hour, 72 * time.Hour
	case "P5":
		return 24 * time.Hour, 168 * time.Hour
	default:
		return 2 * time.Hour, 24 * time.Hour
	}
}

func (w *SLAWorker) sweepCommsStaleness(ctx context.Context, inc store.IncidentRecord) {
	if w.coordinationStore == nil || w.statusUpdateInterval <= 0 {
		return
	}
	if inc.Status == "resolved" || inc.Status == "closed" || inc.Status == "cancelled" {
		return
	}
	incidentID := strconv.FormatInt(inc.IncidentNumber, 10)
	// The "last responder activity" signal is the most recent coordination
	// thread message from an agent. It captures both the explicit
	// `report_to_communicator` comms-task path and the responder's
	// `post_handoff` audience=commander handoff path, so a stale
	// public-update cadence is detected even when the responder skipped
	// `report_to_communicator` (the incident #10 case).
	lastActivity, err := w.coordinationStore.NewestAgentCoordinationReply(ctx, inc.IncidentNumber)
	if err != nil {
		logger.Warn("failed to load newest agent coordination reply", "component", "sla-worker", "incident_id", incidentID, "error", err)
		return
	}
	if lastActivity == nil {
		return
	}
	latest, err := w.coordinationStore.NewestStatusUpdate(ctx, inc.IncidentNumber)
	if err != nil {
		logger.Warn("failed to load newest status update", "component", "sla-worker", "incident_id", incidentID, "error", err)
		return
	}
	if latest != nil && !latest.CreatedAt.Before(lastActivity.CreatedAt) {
		return
	}
	if time.Since(lastActivity.CreatedAt) < w.statusUpdateInterval {
		return
	}
	if !w.markCommsNudgeDeduped(ctx, incidentID) {
		return
	}
	if err := w.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: inc.IncidentNumber,
		EventType:      "comms_stale",
		ActorType:      "system",
		Message:        "Communicator has not published a status update after a responder report",
	}); err != nil {
		logger.Error("failed to add comms_stale timeline entry", "component", "sla-worker", "incident_id", incidentID, "error", err)
	}
	if w.ssePublisher != nil {
		w.ssePublisher.Publish(sse.Event{Type: "incident_comms_stale", Data: map[string]any{"incident_number": inc.IncidentNumber}})
	}
	w.nudgeCommander(ctx, inc)
}

func (w *SLAWorker) markCommsNudgeDeduped(ctx context.Context, incidentID string) bool {
	if w.vkClient == nil {
		return true
	}
	key := fmt.Sprintf("alga:comms:nudge:%s", incidentID)
	ok, err := w.vkClient.SetNX(ctx, key, "1", w.statusUpdateInterval)
	if err != nil {
		logger.Warn("comms nudge dedup SETNX failed", "component", "sla-worker", "key", key, "error", err)
		return true
	}
	return ok
}

func (w *SLAWorker) nudgeCommander(ctx context.Context, inc store.IncidentRecord) {
	if w.icsRoleStore == nil || w.forwarder == nil {
		return
	}
	cmd, err := w.icsRoleStore.GetActiveIC(ctx, inc.IncidentNumber)
	if err != nil || cmd == nil || cmd.AssigneeType != "agent" || cmd.AgentTokenID == nil {
		return
	}
	event := sse.Event{
		Type: "incident_comms_stale",
		Data: map[string]any{
			"incident_number": inc.IncidentNumber,
			"trigger":         "comms_stale",
			"reason":          "communicator has not published a status update after a responder report",
		},
	}
	if err := w.forwarder.ForwardEventToAgent(cmd.AgentTokenID.String(), event); err != nil {
		logger.Warn("failed to forward comms stale nudge to commander", "component", "sla-worker", "incident_number", inc.IncidentNumber, "error", err)
	}
}
