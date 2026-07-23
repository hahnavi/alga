// scheduler_dispatch.go contains the core scheduling pass for both alert and
// incident investigations: filtering online agents, building the capacity
// cache, picking candidates, dispatching to agents, and the backoff /
// resolved-completion bookkeeping that gates re-dispatch.
package worker

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"alga/capability"
	"alga/ics"
	"alga/logger"
	"alga/metrics"
	"alga/prompt"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

// schedule runs one Filter → Score → Bind pass over all pending
// investigations. The pass is intentionally simple: load candidates once,
// score them, then bind the highest-priority pending investigation to the
// best candidate atomically. The atomic claim makes the scoring decision a
// hint rather than a contract — if the DB says the row was already taken we
// just move on.
func (s *InvestigationScheduler) schedule(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	s.nudgeStalled(ctx)
	s.sweepStalledAssigned()
	s.sweepStalledInvestigating()

	agents, err := s.agentTokenStore.ListActiveAgents()
	if err != nil {
		logger.Error("Scheduler failed to list active agents", "component", "scheduler", "error", err)
		return
	}

	online := s.filterOnlineAgents(ctx, agents)
	metrics.SchedulerOnlineAgents.Set(int64(len(online)))
	if len(online) == 0 {
		return
	}

	// Per-agent active count cached for the duration of this tick to avoid
	// O(N*M) DB round-trips: one count query per (agent, score)
	// otherwise.
	capacityCache := s.buildCapacityCache(ctx, online)

	for id := range online {
		if s.healthTracker.IsCircuitBroken(id) {
			delete(online, id)
		}
	}

	pending, err := s.alertInvestigationStore.ListPendingAlertInvestigations(ctx, 200)
	if err != nil {
		logger.Error("Scheduler failed to list pending alert investigations", "component", "scheduler", "error", err)
		return
	}
	metrics.SchedulerPending.Set(int64(len(pending)))

	pending = s.applyBackoff(pending)
	pending = s.filterScope(pending)
	s.completeResolvedInvestigations(ctx, pending)
	pending = filterInactiveAlertInvestigations(s.alertStore, pending)
	if len(pending) == 0 {
		return
	}

	slices.SortStableFunc(pending, func(a, b store.AlertInvestigationRecord) int {
		return int(computePriority(b) - computePriority(a))
	})

	for _, inv := range pending {
		if inv.Status != "pending" {
			continue
		}
		candidate := s.pickCandidate(ctx, inv, online, capacityCache)
		if candidate == nil {
			metrics.SchedulerNoCandidateTotal.Add(1)
			continue
		}

		agentID := candidate.ID.String()
		agentName := candidate.Name
		agentType := candidate.AgentType

		investigation, err := s.alertInvestigationStore.ClaimPendingAlertInvestigation(
			ctx, inv.ID.String(), agentID, agentName, agentType,
		)
		if err != nil {
			logger.Error("Scheduler failed to claim alert investigation", "component", "scheduler", "alert_investigation_id", inv.AlertInvestigationID, "error", err)
			metrics.SchedulerBindFailedTotal.Add(1)
			continue
		}
		if investigation == nil {
			continue
		}

		if s.dispatch(ctx, investigation, candidate) {
			capacityCache[agentID]++
			metrics.SchedulerScheduledTotal.Add(1)
		}
	}
}

func (s *InvestigationScheduler) scheduleIncidentInvestigations(ctx context.Context) {
	if s.incidentInvestigationStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	agents, err := s.agentTokenStore.ListActiveAgents()
	if err != nil {
		logger.Error("Scheduler failed to list active agents for incident investigations", "component", "scheduler", "error", err)
		return
	}

	online := s.filterOnlineAgents(ctx, agents)
	for id := range online {
		if s.healthTracker.IsCircuitBroken(id) {
			delete(online, id)
		}
	}
	if len(online) == 0 {
		return
	}

	capacityCache := s.buildCapacityCache(ctx, online)

	pending, err := s.incidentInvestigationStore.ListPendingIncidentInvestigations(ctx, 100)
	if err != nil {
		logger.Error("Scheduler failed to list pending incident investigations", "component", "scheduler", "error", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	for _, inv := range pending {
		if inv.Status != store.IncidentInvestigationStatusPending {
			continue
		}
		candidate := s.pickIncidentInvestigationCandidate(ctx, inv.IncidentNumber, online, capacityCache)
		if candidate == nil {
			continue
		}

		agentID := candidate.ID.String()
		agentName := candidate.Name
		agentType := candidate.AgentType

		claimed, claimErr := s.incidentInvestigationStore.ClaimPendingIncidentInvestigation(
			ctx, inv.IncidentInvestigationID, agentID, agentName, agentType,
		)
		if claimErr != nil {
			logger.Error("Scheduler failed to claim incident investigation", "component", "scheduler", "incident_investigation_id", inv.IncidentInvestigationID, "error", claimErr)
			continue
		}
		if claimed == nil {
			continue
		}

		if s.dispatchIncidentInvestigation(ctx, claimed, candidate) {
			capacityCache[agentID]++
		}
	}
}

func (s *InvestigationScheduler) pickIncidentInvestigationCandidate(ctx context.Context, incidentNumber int64, online map[string]*store.AgentTokenRecord, capacityCache map[string]int) *store.AgentTokenRecord {
	available := make(map[string]*store.AgentTokenRecord, len(online))
	for id, a := range online {
		if capability.Has(a.Capabilities, capability.Investigate) && capacityCache[id] < s.maxConcurrent {
			// Exclude the incident commander from regular investigation dispatch —
			// the commander is a pure orchestrator and must not execute
			// investigative work. An agent with no ICS role yet (""") is allowed.
			if role := s.resolveIncidentRole(ctx, incidentNumber, id); role == string(ics.RoleIncidentCommander) {
				continue
			}
			available[id] = a
		}
	}
	return pickLeastLoaded(available, capacityCache)
}

func (s *InvestigationScheduler) resolveIncidentRole(ctx context.Context, incidentNumber int64, agentID string) string {
	if s.icsRoleStore == nil || incidentNumber == 0 || agentID == "" {
		return ""
	}
	roles, err := s.icsRoleStore.GetActiveRoles(ctx, incidentNumber)
	if err != nil {
		logger.Warn("Scheduler failed to resolve incident role for agent", "component", "scheduler", "incident_number", incidentNumber, "agent_id", agentID, "error", err)
		return ""
	}
	for _, role := range roles {
		if role.Status == "active" && role.AssigneeType == "agent" && role.AgentTokenID != nil && role.AgentTokenID.String() == agentID {
			return role.RoleType
		}
	}
	return ""
}

func (s *InvestigationScheduler) dispatchIncidentInvestigation(ctx context.Context, inv *store.IncidentInvestigationRecord, agent *store.AgentTokenRecord) bool {
	agentID := agent.ID.String()
	invID := inv.IncidentInvestigationID

	input := prompt.DispatchInput{
		InvestigationID:      invID,
		InvestigationTimeout: s.investigationTimeout,
	}
	if inv.IncidentNumber != 0 {
		input.IncidentScope = true
		input.IncidentID = strconv.FormatInt(inv.IncidentNumber, 10)
		input.IncidentRole = s.resolveIncidentRole(ctx, inv.IncidentNumber, agentID)
		// Populate the incident lifecycle status so the dispatch prompt can
		// inject a phase-aware checklist for the commander (orchestrator).
		if s.incidentStore != nil {
			if inc, incErr := s.incidentStore.GetIncident(ctx, inv.IncidentNumber); incErr == nil && inc != nil {
				input.IncidentStatus = inc.Status
			}
		}
	}
	s.enrichWithOpsTeam(ctx, &input)
	p := s.buildDispatchPrompt(ctx, input)

	if err := s.resolver.ForwardToAgent(agentID, invID, "system", "System", p); err != nil {
		logger.Error("Scheduler failed to forward incident investigation to agent", "component", "scheduler", "incident_investigation_id", invID, "agent_name", agent.Name, "error", err)
		_ = s.incidentInvestigationStore.UpdateIncidentInvestigationStatus(ctx, invID, store.IncidentInvestigationStatusPending)
		s.healthTracker.RecordFailure(agentID)
		return false
	}

	if err := s.incidentInvestigationStore.UpdateIncidentInvestigationStatus(ctx, invID, store.IncidentInvestigationStatusInvestigating); err != nil {
		logger.Warn("Scheduler failed to transition incident investigation to investigating", "component", "scheduler", "incident_investigation_id", invID, "error", err)
	}

	s.healthTracker.RecordSuccess(agentID)
	logger.Info("Scheduler assigned incident investigation to agent", "component", "scheduler", "incident_investigation_id", invID, "agent_name", agent.Name, "agent_id", agentID)

	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{
			Type: "investigation_status_changed",
			Data: map[string]any{
				"incident_investigation_id": invID,
				"status":                    store.IncidentInvestigationStatusInvestigating,
				"agent_name":                agent.Name,
				"agent_type":                agent.AgentType,
			},
		})
	}
	if inv.IncidentNumber != 0 {
		s.postInvestigatingStatusUpdate(ctx, inv.IncidentNumber)
	}
	return true
}

// postInvestigatingStatusUpdate auto-posts an "investigating" status update to
// the incident's Status Updates card when an agent begins investigating. It is
// best-effort: failures only emit a structured warning and never fail the
// dispatch. It is skipped when the newest status update is already
// "investigating" so reassignment and pause/resume do not spam the card.
func (s *InvestigationScheduler) postInvestigatingStatusUpdate(ctx context.Context, incidentNumber int64) {
	if s.incidentCoordinationStore == nil || incidentNumber == 0 {
		return
	}
	newest, err := s.incidentCoordinationStore.NewestStatusUpdate(ctx, incidentNumber)
	if err != nil {
		logger.Warn("Scheduler failed to check newest status update before auto-post", "component", "scheduler", "incident_number", incidentNumber, "error", err)
		return
	}
	if newest != nil {
		if level, _ := newest.Metadata["status_level"].(string); level == "investigating" {
			return
		}
	}
	const body = "We're investigating this incident and will share an update as soon as we know more."
	created, err := s.incidentCoordinationStore.CreateMessage(ctx, &store.IncidentCoordinationMessageRecord{
		IncidentNumber:   incidentNumber,
		Kind:             store.IncidentCoordinationKindStatusUpdate,
		ActorType:        store.IncidentCoordinationActorSystem,
		ActorDisplayName: "System",
		Source:           store.IncidentCoordinationSourceSystem,
		Body:             body,
		Internal:         false,
		Metadata:         map[string]any{"status_level": "investigating", "auto": true},
	})
	if err != nil {
		logger.Warn("Scheduler failed to auto-post investigating status update", "component", "scheduler", "incident_number", incidentNumber, "error", err)
		return
	}
	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{
			Type: "incident_updated",
			Data: map[string]string{"incident_number": strconv.FormatInt(incidentNumber, 10), "action": "status_update"},
		})
	}
	if s.incidentStore != nil {
		if err := s.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
			IncidentNumber: incidentNumber,
			EventType:      "status_update",
			ActorType:      "system",
			Message:        "Automated investigating status update posted",
		}); err != nil {
			logger.Warn("Scheduler failed to add timeline entry for investigating status update", "component", "scheduler", "incident_number", incidentNumber, "error", err)
		}
	}
	if s.auditStore != nil {
		messageID := ""
		if created != nil {
			messageID = created.ID.String()
		}
		s.auditStore.Log(store.AuditIncidentStatusUpdateCreated, nil, "System", "", "", true, map[string]any{
			"incident_number": incidentNumber,
			"message_id":      messageID,
			"auto":            true,
		})
	}
}

// filterOnlineAgents drops agents that aren't currently reachable. We
// prefer Valkey-backed presence (so any replica's SSE counts) and fall back
// to the resolver-local view when Valkey is absent or returns empty.
func (s *InvestigationScheduler) filterOnlineAgents(ctx context.Context, agents []store.AgentTokenRecord) map[string]*store.AgentTokenRecord {
	out := make(map[string]*store.AgentTokenRecord, len(agents))

	useValkey := s.presence != nil && s.presence.Available()
	for i := range agents {
		id := agents[i].ID.String()
		online := false
		if useValkey {
			online = s.presence.IsAgentOnline(ctx, id)
		}
		if !online && s.resolver != nil {
			online = s.resolver.AgentOnline(id)
		}
		if online {
			out[id] = &agents[i]
		}
	}
	return out
}

// buildCapacityCache computes per-agent active counts in a single DB
// round-trip per agent (still O(N) instead of O(N*M)). The cache is used
// throughout the rest of the tick.
func (s *InvestigationScheduler) buildCapacityCache(ctx context.Context, online map[string]*store.AgentTokenRecord) map[string]int {
	ids := make([]string, 0, len(online))
	for id := range online {
		ids = append(ids, id)
	}
	if s.alertInvestigationStore == nil {
		metrics.SchedulerAgentCapacityMax.Set(int64(s.maxConcurrent * len(online)))
		metrics.SchedulerAgentCapacityUse.Set(0)
		return make(map[string]int, len(ids))
	}
	cache, err := s.alertInvestigationStore.CountActiveByAgents(ctx, ids)
	if err != nil {
		logger.Error("Scheduler batch capacity query failed", "component", "scheduler", "error", err)
		cache = make(map[string]int, len(ids))
	}
	metrics.SchedulerAgentCapacityMax.Set(int64(s.maxConcurrent * len(online)))
	used := 0
	for _, n := range cache {
		used += n
	}
	metrics.SchedulerAgentCapacityUse.Set(int64(used))
	return cache
}

func (s *InvestigationScheduler) filterOnlineAgentsByCapability(ctx context.Context, agents []store.AgentTokenRecord, requiredCap string) map[string]*store.AgentTokenRecord {
	online := s.filterOnlineAgents(ctx, agents)
	filtered := make(map[string]*store.AgentTokenRecord, len(online))
	for id, a := range online {
		if capability.Has(a.Capabilities, requiredCap) {
			filtered[id] = a
		}
	}
	return filtered
}

func pickLeastLoaded(agents map[string]*store.AgentTokenRecord, capacityCache map[string]int) *store.AgentTokenRecord {
	var best *store.AgentTokenRecord
	bestLoad := -1
	for id, a := range agents {
		load := capacityCache[id]
		if best == nil || load < bestLoad {
			best = a
			bestLoad = load
		}
	}
	return best
}

// applyBackoff removes investigations that recently failed to forward.
// The map is opportunistically pruned in the same pass.
//
// The pending slice is reused via pending[:0]; the in-place modification
// of s.backoff[inv.AlertInvestigationID] is intentional.
func (s *InvestigationScheduler) applyBackoff(pending []store.AlertInvestigationRecord) []store.AlertInvestigationRecord {
	s.backoffMu.Lock()
	defer s.backoffMu.Unlock()
	now := time.Now()
	out := pending[:0]
	skipped := 0
	for _, inv := range pending {
		if until, ok := s.backoff[inv.AlertInvestigationID]; ok {
			if now.Before(until) {
				skipped++
				continue
			}
			delete(s.backoff, inv.AlertInvestigationID)
		}
		out = append(out, inv)
	}
	if skipped > 0 {
		metrics.SchedulerSkipActiveBackoffTotal.Add(int64(skipped))
	}
	return out
}

// filterScope drops incident-scoped investigations that are assigned to a
// human (user or team). The scheduler only picks up:
//   - scope=alert investigations (default alert-driven investigations)
//   - scope=incident with assignee_type=agent (agent-driven incident work)
//
// Human-driven incident investigations (assignee_type=user or team) are
// skipped so the scheduler does not attempt to claim them.
func (s *InvestigationScheduler) filterScope(pending []store.AlertInvestigationRecord) []store.AlertInvestigationRecord {
	out := pending[:0]
	for _, inv := range pending {
		if inv.AssigneeType != "" && inv.AssigneeType != "agent" {
			continue
		}
		out = append(out, inv)
	}
	return out
}

// alertFingerprintLookup intentionally resolves via GetOpenByFingerprint so a
// reused fingerprint (a previously-resolved alert + a new firing alert with the
// same fingerprint) returns the live open alert rather than the soft-deleted
// tombstone. alert_number remains the canonical identity per the domain
// invariants; this lookup is only used to confirm the investigation's primary
// alert still has an open, non-deleted row.
type alertFingerprintLookup interface {
	GetOpenByFingerprint(fingerprint string) (*store.AlertRecord, error)
}

func filterInactiveAlertInvestigations(checker alertFingerprintLookup, pending []store.AlertInvestigationRecord) []store.AlertInvestigationRecord {
	if checker == nil {
		return pending
	}

	out := pending[:0]
	for _, inv := range pending {
		if len(inv.Alerts) == 0 {
			out = append(out, inv)
			continue
		}
		if hasCurrentActiveAlert(checker, inv.Alerts) {
			out = append(out, inv)
		}
	}
	return out
}

func (s *InvestigationScheduler) completeResolvedInvestigations(ctx context.Context, pending []store.AlertInvestigationRecord) {
	if s.alertInvestigationLifecycle == nil || s.alertStore == nil {
		return
	}
	for _, inv := range pending {
		if len(inv.Alerts) == 0 {
			continue
		}
		if inv.Status != "pending" && inv.Status != "assigned" && inv.Status != "investigating" {
			continue
		}
		for _, a := range inv.Alerts {
			if a.AlertNumber <= 0 {
				continue
			}
			if err := s.alertInvestigationLifecycle.CompleteIfAllAlertsResolved(ctx, store.AlertInvestigationLifecycleCompletionRequest{
				AlertNumber: a.AlertNumber,
				Reason:      store.AlertInvestigationCompletedReasonAlertsResolved,
				ActorType:   store.InvestigationActorSystem,
				ActorName:   "scheduler",
			}); err != nil {
				logger.Warn("scheduler: auto-complete lifecycle failed", "component", "scheduler", "alert_number", a.AlertNumber, "error", err)
			}
		}
	}
}

func hasCurrentActiveAlert(checker alertFingerprintLookup, alerts []rabbitmq.CorrelatedAlert) bool {
	for _, a := range alerts {
		if strings.TrimSpace(a.Fingerprint) == "" {
			return true
		}
		// GetOpenByFingerprint filters status=resolved and deleted_at IS NOT NULL,
		// so a fingerprint reused by a new firing alert resolves to that live alert
		// even if a previous alert with the same fingerprint is soft-deleted.
		current, err := checker.GetOpenByFingerprint(a.Fingerprint)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return true
		}
		if current != nil {
			return true
		}
	}
	return false
}

// pickCandidate runs Filter (online + capability + capacity + selector match) → Score
// (specificity, then spread by least-loaded) and returns the winning
// agent, or nil when no agent matches.
func (s *InvestigationScheduler) pickCandidate(ctx context.Context, inv store.AlertInvestigationRecord, online map[string]*store.AgentTokenRecord, capacityCache map[string]int) *store.AgentTokenRecord {
	labels := extractLabels(inv)

	type scored struct {
		agent      *store.AgentTokenRecord
		score      int
		active     int
		isCatchAll bool
	}
	var labelMatches []scored
	var catchAlls []scored

	for _, agent := range online {
		if !capability.Has(agent.Capabilities, capability.Investigate) {
			continue
		}
		used := capacityCache[agent.ID.String()]
		if used >= s.maxConcurrent {
			continue
		}
		scope := agent.Scope
		if scope == "" {
			scope = "all"
		}
		if scope == "all" {
			catchAlls = append(catchAlls, scored{agent: agent, active: used, isCatchAll: true})
			continue
		}
		if scope == "labels" && s.matchConditions(agent.LabelSelectors, labels) {
			labelMatches = append(labelMatches, scored{
				agent:  agent,
				score:  computeSpecificity(agent.LabelSelectors),
				active: used,
			})
		}
	}

	// Prefer label-targeted agents over catch-all so a "frontend agent"
	// with a label selector beats a generic "all" agent for frontend
	// alerts. Within the chosen tier, sort by score desc, then active asc
	// (spread), then ID for stability.
	pick := func(pool []scored) *store.AgentTokenRecord {
		if len(pool) == 0 {
			return nil
		}
		slices.SortStableFunc(pool, func(a, b scored) int {
			if a.score != b.score {
				return int(b.score - a.score)
			}
			if a.active != b.active {
				if a.active < b.active {
					return 1
				}
				return -1
			}
			healthA := s.healthTracker.Health(a.agent.ID.String())
			healthB := s.healthTracker.Health(b.agent.ID.String())
			if healthA != healthB {
				return int(healthB - healthA)
			}
			if a.agent.ID.String() < b.agent.ID.String() {
				return -1
			}
			if a.agent.ID.String() > b.agent.ID.String() {
				return 1
			}
			return 0
		})
		return pool[0].agent
	}

	if a := pick(labelMatches); a != nil {
		return a
	}
	return pick(catchAlls)
}

// dispatch forwards the assigned investigation to the chosen agent and
// updates Valkey active-investigation registry. On forward failure the
// investigation is reset to pending and an exponential backoff is recorded
// so we don't immediately re-pick the same target.
func (s *InvestigationScheduler) dispatch(ctx context.Context, investigation *store.AlertInvestigationRecord, agent *store.AgentTokenRecord) bool {
	invID := investigation.AlertInvestigationID
	s.dispatchMu.Lock()
	prev := s.dispatchAttempts[invID]
	s.dispatchAttempts[invID] = dispatchAttempt{count: prev.count + 1, lastSeen: time.Now()}
	attempts := s.dispatchAttempts[invID].count
	s.dispatchMu.Unlock()

	if attempts > maxDispatchAttempts {
		logger.Warn("Scheduler investigation exceeded max dispatch attempts; marking as timed_out", "component", "scheduler", "alert_investigation_id", invID, "attempts", attempts)
		_ = s.alertInvestigationStore.UpdateAlertInvestigationStatus(ctx, invID, "timed_out")
		_ = s.alertInvestigationStore.AddAlertInvestigationUpdate(ctx, invID, store.InvestigationUpdate{
			Type:     store.UpdateTypeDeadLetter,
			Message:  fmt.Sprintf("Investigation dead-lettered after %d dispatch attempts - no agent could accept it", attempts),
			Source:   store.UpdateSourceSystem,
			Internal: true,
		})
		metrics.SchedulerDLQTotal.Add(1)
		s.dispatchMu.Lock()
		delete(s.dispatchAttempts, invID)
		s.dispatchMu.Unlock()
		s.nudged.Delete(invID)
		return false
	}

	agentID := agent.ID.String()
	input := prompt.DispatchInput{
		InvestigationID:         invID,
		InvestigationTimeout:    s.investigationTimeout,
		Alerts:                  investigation.Alerts,
		Severity:                rabbitmq.DetermineAlertSeverity(investigation.Alerts),
		CorrelationKey:          investigation.CorrelationKey,
		PrimaryAlertFingerprint: investigation.PrimaryAlertFingerprint,
		PrimaryAlertNumber:      investigation.PrimaryAlertNumber,
	}
	if investigation.PromotedIncidentID != nil {
		input.IncidentScope = true
		input.IncidentID = investigation.PromotedIncidentID.String()
		input.PromotedAlertHandoff = true
		if s.incidentStore != nil {
			if inc, err := s.incidentStore.GetIncidentByID(ctx, *investigation.PromotedIncidentID); err == nil && inc != nil {
				input.IncidentNumber = inc.IncidentNumber
				input.IncidentRole = s.resolveIncidentRole(ctx, inc.IncidentNumber, agentID)
			}
		}
	}
	if investigation.PromotedIncidentInvestigationID != nil {
		input.PromotedIncidentInvestigationID = investigation.PromotedIncidentInvestigationID.String()
	}
	s.enrichWithOpsTeam(ctx, &input)
	p := s.buildDispatchPrompt(ctx, input)
	dispatchStart := time.Now()
	if err := s.resolver.ForwardToAgent(agentID, invID, "system", "System", p); err != nil {
		logger.Error("Scheduler failed to forward investigation to agent", "component", "scheduler", "alert_investigation_id", invID, "agent_name", agent.Name, "error", err)
		_ = s.alertInvestigationStore.TransitionAlertInvestigationStatus(ctx, investigation.ID.String(), []string{"assigned"}, "pending")
		s.recordBackoff(invID, attempts)
		s.healthTracker.RecordFailure(agentID)
		metrics.SchedulerBindFailedTotal.Add(1)
		return false
	}
	s.autoAcknowledge(ctx, investigation)
	s.registerActiveInvestigation(ctx, investigation)
	s.healthTracker.RecordSuccess(agentID)
	metrics.SchedulerDispatchLatencyMs.Set(time.Since(dispatchStart).Milliseconds())
	s.dispatchMu.Lock()
	delete(s.dispatchAttempts, invID)
	s.dispatchMu.Unlock()
	s.nudged.Delete(invID)
	logger.Info("Scheduler assigned investigation to agent", "component", "scheduler", "alert_investigation_id", invID, "agent_name", agent.Name, "agent_id", agentID, "attempt", attempts, "max_attempts", maxDispatchAttempts)

	if s.ssePublisher != nil {
		s.ssePublisher.Publish(sse.Event{
			Type: "investigation_status_changed",
			Data: map[string]any{
				"alert_investigation_id": invID,
				"status":                 "assigned",
				"agent_name":             agent.Name,
				"agent_type":             store.NormalizeAgentType(agent.AgentType),
			},
		})
	}

	return true
}

func (s *InvestigationScheduler) autoAcknowledge(ctx context.Context, inv *store.AlertInvestigationRecord) {
	if inv == nil {
		return
	}
	actor := &store.EventActor{Username: "System", Source: "system"}
	if s.alertStore != nil {
		for _, a := range inv.Alerts {
			if err := s.alertStore.AcknowledgeAlert(a.Fingerprint, actor); err != nil {
				logger.Warn("Scheduler auto-acknowledge failed for alert", "component", "scheduler", "fingerprint", a.Fingerprint, "error", err)
			} else if s.ssePublisher != nil {
				if rec, err := s.alertStore.GetByFingerprint(a.Fingerprint); err == nil {
					s.ssePublisher.Publish(sse.Event{
						Type: "alert_updated",
						Data: *rec,
					})
				} else {
					logger.Warn("Scheduler failed to fetch alert after auto-acknowledge", "component", "scheduler", "fingerprint", a.Fingerprint, "error", err)
				}
			}
		}
	}
	if inv.PromotedIncidentID != nil && s.incidentStore != nil {
		ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if inc, err := s.incidentStore.GetIncidentByID(ackCtx, *inv.PromotedIncidentID); err == nil && inc != nil {
			// Align with the operator handleAcknowledgeIncident path, which
			// transitions detected -> active. "acknowledged" is not part of
			// the documented incident lifecycle and is never read elsewhere.
			if err := s.incidentStore.TransitionIncidentStatus(ackCtx, inc.IncidentNumber, []string{"detected", "triaging"}, "active"); err != nil && !errors.Is(err, store.ErrIncidentStatusConflict) {
				logger.Warn("Scheduler auto-acknowledge failed for incident", "component", "scheduler", "incident_number", inc.IncidentNumber, "error", err)
			}
		}
		cancel()
	}
}

const maxBackoff = 480 * time.Second

func backoffDuration(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	exp := uint(min(attempt-1, 4)) //#nosec G115 -- value clamped to [0,4] by min(_,4)
	d := failureBackoff * time.Duration(1<<exp)
	d = min(d, maxBackoff)
	return d
}

func (s *InvestigationScheduler) recordBackoff(invID string, attempt int) {
	s.backoffMu.Lock()
	defer s.backoffMu.Unlock()
	s.backoff[invID] = time.Now().Add(backoffDuration(attempt))
}

func (s *InvestigationScheduler) clearBackoff(invID string) {
	s.backoffMu.Lock()
	defer s.backoffMu.Unlock()
	delete(s.backoff, invID)
}

// registerActiveInvestigation records an in-flight investigation in Valkey so
// peer agents can surface overlap in their CONCURRENT INVESTIGATIONS block.
// Failures are logged but do not interrupt scheduling; TTL acts as the
// correctness backstop when the agent crashes mid-run.
func (s *InvestigationScheduler) registerActiveInvestigation(ctx context.Context, inv *store.AlertInvestigationRecord) {
	if s.valkeyClient == nil || inv == nil {
		return
	}
	info := valkey.ActiveInvestigation{
		InvestigationID: inv.AlertInvestigationID,
		AgentID:         inv.AgentID,
		AgentType:       inv.AgentType,
		Severity:        rabbitmq.DetermineAlertSeverity(inv.Alerts),
		StartedAt:       time.Now().UTC(),
	}
	var discriminators map[string]string
	if len(inv.Alerts) > 0 {
		info.AlertName = inv.Alerts[0].Labels["alertname"]
		info.Namespace = inv.Alerts[0].Labels["namespace"]
		discriminators = buildDiscriminators(inv.Alerts[0].Labels)
	}
	ttl := s.activeTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if err := s.valkeyClient.RegisterActiveInvestigation(ctx, info, discriminators, ttl); err != nil {
		logger.Warn("Scheduler failed to register active investigation", "component", "scheduler", "alert_investigation_id", inv.AlertInvestigationID, "error", err)
	}
}

func (s *InvestigationScheduler) NotifyPending() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *InvestigationScheduler) CleanupCompleted(investigationID string) {
	s.nudged.Delete(investigationID)
	s.dispatchMu.Lock()
	delete(s.dispatchAttempts, investigationID)
	s.dispatchMu.Unlock()
	s.clearBackoff(investigationID)
}
