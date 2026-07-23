package worker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/store"
)

const (
	stuckInvEscDedupPrefix = "alga:esc:inv:"
	stuckInvEscDedupTTL    = time.Hour
)

// stuckInvestigationLister is the minimal store interface the stuck
// investigation worker needs. Defining it locally keeps the worker decoupled
// from the full AlertInvestigationStore surface and makes the fakes tiny.
type stuckInvestigationLister interface {
	ListStalledAssignedAlertInvestigations(ctx context.Context, threshold time.Duration) ([]store.AlertInvestigationRecord, error)
	ListStalledInvestigatingAlertInvestigations(ctx context.Context, threshold time.Duration) ([]store.AlertInvestigationRecord, error)
}

type stuckIncidentReader interface {
	GetIncidentByID(ctx context.Context, id uuid.UUID) (*store.IncidentRecord, error)
	AddTimelineEntry(ctx context.Context, record *store.IncidentTimelineEntryRecord) error
}

type stuckServiceReader interface {
	GetService(ctx context.Context, id string) (*store.ServiceRecord, error)
}

type stuckTeamReader interface {
	GetTeamByName(ctx context.Context, name string) (*store.TeamRecord, error)
}

type stuckEscalationPublisher interface {
	PublishEscalation(ctx context.Context, msg rabbitmq.EscalationMessage) error
}

type stuckDedupStore interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key, field, value string) error
}

type StuckInvestigationEscalationWorker struct {
	alertInvestigationStore stuckInvestigationLister
	incidentStore           stuckIncidentReader
	serviceStore            stuckServiceReader
	teamStore               stuckTeamReader
	publisher               stuckEscalationPublisher
	vkClient                stuckDedupStore
	multiplier              int
	tickInterval            time.Duration
	investigationTimeout    time.Duration
	opsTeamName             string
}

func NewStuckInvestigationEscalationWorker(
	alertInvestigationStore stuckInvestigationLister,
	incidentStore stuckIncidentReader,
	serviceStore stuckServiceReader,
	teamStore stuckTeamReader,
	publisher stuckEscalationPublisher,
	vkClient stuckDedupStore,
	multiplier int,
	tickInterval time.Duration,
	investigationTimeout time.Duration,
	opsTeamName string,
) *StuckInvestigationEscalationWorker {
	multiplier = max(multiplier, 0)
	if tickInterval <= 0 {
		tickInterval = 30 * time.Second
	}
	if investigationTimeout <= 0 {
		investigationTimeout = 10 * time.Minute
	}
	if opsTeamName == "" {
		opsTeamName = "ops-team"
	}
	return &StuckInvestigationEscalationWorker{
		alertInvestigationStore: alertInvestigationStore,
		incidentStore:           incidentStore,
		serviceStore:            serviceStore,
		teamStore:               teamStore,
		publisher:               publisher,
		vkClient:                vkClient,
		multiplier:              multiplier,
		tickInterval:            tickInterval,
		investigationTimeout:    investigationTimeout,
		opsTeamName:             opsTeamName,
	}
}

func (w *StuckInvestigationEscalationWorker) Run(ctx context.Context) {
	if w.multiplier == 0 {
		logger.Info("Stuck investigation escalation disabled (multiplier=0)", "component", "stuck-investigation-escalation")
		return
	}
	runTickerLoop(ctx, w.tickInterval, "stuck-investigation-escalation", w.tick)
}

func (w *StuckInvestigationEscalationWorker) tick(ctx context.Context) {
	if w.alertInvestigationStore == nil {
		return
	}
	threshold := time.Duration(w.multiplier) * w.investigationTimeout
	if threshold <= 0 {
		return
	}

	invLists := []func(context.Context, time.Duration) ([]store.AlertInvestigationRecord, error){
		w.alertInvestigationStore.ListStalledAssignedAlertInvestigations,
		w.alertInvestigationStore.ListStalledInvestigatingAlertInvestigations,
	}

	for _, listFn := range invLists {
		invs, err := listFn(ctx, threshold)
		if err != nil {
			logger.Error("Stuck investigation escalation: failed to list stalled investigations", "component", "stuck-investigation-escalation", "error", err)
			continue
		}
		for _, inv := range invs {
			w.processOne(ctx, inv)
		}
	}
}

func (w *StuckInvestigationEscalationWorker) processOne(ctx context.Context, inv store.AlertInvestigationRecord) {
	if w.claimDedup(ctx, inv.AlertInvestigationID) {
		w.escalate(ctx, inv)
	}
}

func (w *StuckInvestigationEscalationWorker) claimDedup(ctx context.Context, alertInvestigationID string) bool {
	if w.vkClient == nil || alertInvestigationID == "" {
		return true
	}
	set, err := w.vkClient.SetNX(ctx, stuckInvEscDedupPrefix+alertInvestigationID, "1", stuckInvEscDedupTTL)
	if err != nil {
		logger.Warn("Stuck investigation escalation: dedup setnx failed, proceeding", "component", "stuck-investigation-escalation", "alert_investigation_id", alertInvestigationID, "error", err)
		return true
	}
	return set
}

func (w *StuckInvestigationEscalationWorker) escalate(ctx context.Context, inv store.AlertInvestigationRecord) {
	policyID, ok := w.resolvePolicyID(ctx, inv)
	if !ok {
		logger.Info("Stuck investigation escalation: no policy available, skipping", "component", "stuck-investigation-escalation", "alert_investigation_id", inv.AlertInvestigationID)
		return
	}

	var incidentNumber int64
	if inv.PromotedIncidentID != nil && w.incidentStore != nil {
		if inc, err := w.incidentStore.GetIncidentByID(ctx, *inv.PromotedIncidentID); err == nil && inc != nil {
			incidentNumber = inc.IncidentNumber
		}
	}

	// Suppress if the user has already acknowledged or silenced this incident.
	if incidentNumber > 0 {
		hashKey := escHashPrefix + strconv.FormatInt(incidentNumber, 10)
		if verdict := preflightEscalationState(ctx, w.vkClient, hashKey); verdict != guardAllow {
			logger.Info("Stuck investigation escalation suppressed by guard", "component", "stuck-investigation-escalation", "alert_investigation_id", inv.AlertInvestigationID, "incident_number", incidentNumber, "reason", verdict.String())
			return
		}
	}

	if w.publisher != nil {
		// Use the same atomic claim the other publishers use so two stuck
		// investigations, or a stuck investigation racing autoAssignIC, do
		// not both publish an EscalationMessage for the same incident.
		if incidentNumber > 0 {
			verdict := claimEscalationFirstPublish(ctx, w.vkClient, incidentNumber, policyID.String(), 1)
			if verdict != guardAllow {
				logger.Info("Stuck investigation escalation suppressed by claim", "component", "stuck-investigation-escalation", "alert_investigation_id", inv.AlertInvestigationID, "incident_number", incidentNumber, "reason", verdict.String())
				return
			}
		}
		msg := rabbitmq.EscalationMessage{
			IncidentNumber: incidentNumber,
			PolicyID:       policyID,
			Level:          1,
			MaxRetries:     rabbitmq.MaxEscalationRetries,
		}
		if perr := w.publisher.PublishEscalation(ctx, msg); perr != nil {
			logger.Error("Stuck investigation escalation: failed to publish", "component", "stuck-investigation-escalation", "alert_investigation_id", inv.AlertInvestigationID, "error", perr)
			return
		}
	}

	metrics.StuckInvestigationsEscalated.Add(1)
	logger.Info("Stuck investigation escalation fired", "component", "stuck-investigation-escalation", "alert_investigation_id", inv.AlertInvestigationID, "policy_id", policyID, "incident_number", incidentNumber)

	if incidentNumber > 0 && w.incidentStore != nil {
		elapsed := ""
		if inv.StartedAt != nil {
			elapsed = time.Since(*inv.StartedAt).Truncate(time.Second).String()
		}
		entry := &store.IncidentTimelineEntryRecord{
			IncidentNumber: incidentNumber,
			EventType:      "stuck_investigation_escalation",
			ActorType:      "system",
			Message:        fmt.Sprintf("Investigation %s stuck for %s; paging on-call", inv.AlertInvestigationID, elapsed),
			Metadata: map[string]any{
				"alert_investigation_id":  inv.AlertInvestigationID,
				"elapsed":                 elapsed,
				"escalation_policy_id":    policyID.String(),
				"stuck_threshold_seconds": int64(time.Duration(w.multiplier) * w.investigationTimeout / time.Second),
			},
		}
		if err := w.incidentStore.AddTimelineEntry(ctx, entry); err != nil {
			logger.Warn("Stuck investigation escalation: failed to add timeline entry", "component", "stuck-investigation-escalation", "incident_number", incidentNumber, "error", err)
		}
	}
}

// resolvePolicyID returns the escalation policy to page for a stuck
// investigation. It prefers the promoted incident's direct policy, then the
// service's policy. A missing policy is a valid result; callers skip
// escalation when no policy is configured on the incident or its service.
func (w *StuckInvestigationEscalationWorker) resolvePolicyID(ctx context.Context, inv store.AlertInvestigationRecord) (uuid.UUID, bool) {
	if w.incidentStore != nil && inv.PromotedIncidentID != nil {
		if inc, err := w.incidentStore.GetIncidentByID(ctx, *inv.PromotedIncidentID); err == nil && inc != nil {
			if inc.EscalationPolicyID != nil {
				return *inc.EscalationPolicyID, true
			}
			if inc.ServiceID != nil && w.serviceStore != nil {
				if svc, serr := w.serviceStore.GetService(ctx, inc.ServiceID.String()); serr == nil && svc != nil && svc.EscalationPolicyID != nil {
					return *svc.EscalationPolicyID, true
				}
			}
		}
	}
	return uuid.Nil, false
}
