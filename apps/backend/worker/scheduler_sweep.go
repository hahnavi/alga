// scheduler_sweep.go contains the long-running background loops and their
// per-tick helpers: stale-alert sweep, stalled-investigation resets, nudge
// re-dispatch, data-retention prune, incident sweep, summary sweep, on-call
// handoff tick, the dispatch-attempt map purge, and the prompt/label helper
// functions that feed them.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/config"
	"alga/correlator"
	"alga/ics"
	"alga/logger"
	"alga/matching"
	"alga/metrics"
	"alga/prompt"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
)

func (s *InvestigationScheduler) runMapPurge() {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "map_purge", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.nudged.Range(func(key, _ any) bool {
					s.nudged.Delete(key)
					return true
				})
				now := time.Now()
				s.purgeDispatchAttempts()
				s.backoffMu.Lock()
				for k, t := range s.backoff {
					if now.After(t) {
						delete(s.backoff, k)
					}
				}
				s.backoffMu.Unlock()
			}()
		}
	}
}

// purgeDispatchAttempts removes dispatchAttempts entries that haven't been
// touched in more than 2*investigationTimeout, preserving in-flight retries.
// Extracted from runMapPurge so the retention policy is unit-testable
// without waiting for the 5-minute ticker.
func (s *InvestigationScheduler) purgeDispatchAttempts() {
	timeout := s.investigationTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	s.dispatchMu.Lock()
	for k, v := range s.dispatchAttempts {
		if time.Since(v.lastSeen) > 2*timeout {
			delete(s.dispatchAttempts, k)
		}
	}
	s.dispatchMu.Unlock()
}

func (s *InvestigationScheduler) resetStalledByStatus(resetFn func(time.Duration) ([]string, error), label string) {
	timeout := s.investigationTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ids, err := resetFn(timeout)
	if err != nil {
		logger.Error("Scheduler failed to sweep stalled investigations", "component", "scheduler", "status", label, "error", err)
		return
	}
	if len(ids) > 0 {
		for _, id := range ids {
			s.nudged.Delete(id)
			s.clearBackoff(id)
		}
		s.dispatchMu.Lock()
		for _, id := range ids {
			delete(s.dispatchAttempts, id)
		}
		s.dispatchMu.Unlock()
		logger.Info("Scheduler reset stalled investigations", "component", "scheduler", "count", len(ids), "status", label, "timeout", timeout)
		s.NotifyPending()
	}
}

func (s *InvestigationScheduler) sweepStalledAssigned() {
	s.resetStalledByStatus(s.alertInvestigationStore.ResetStalledAssignedAlertInvestigations, "assigned")
}

func (s *InvestigationScheduler) sweepStalledInvestigating() {
	s.resetStalledByStatus(s.alertInvestigationStore.ResetStalledInvestigatingAlertInvestigations, "investigating")
}

// runStaleSweep is the long-running goroutine that periodically sweeps for
// firing alerts without investigations and publishes investigation jobs.
// It runs on a slower cadence than the main scheduling tick and is gated by
// the same leader lease when HA is configured.
func (s *InvestigationScheduler) runStaleSweep() {
	defer s.wg.Done()
	interval := s.staleInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "stale_sweep", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.staleSweepTick(context.Background())
			}()
		}
	}
}

// staleSweepTick runs one pass of the stale alert sweep. Only the leader runs
// the sweep; non-leader replicas skip silently.
func (s *InvestigationScheduler) staleSweepTick(ctx context.Context) {
	if !s.acquireLeadership(ctx) {
		return
	}

	metrics.SchedulerStaleSweepTickTotal.Add(1)

	alerts, err := s.alertStore.ListUninvestigatedAlerts(ctx, s.staleThreshold)
	if err != nil {
		logger.Error("Scheduler stale sweep failed to list uninvestigated alerts", "component", "scheduler", "error", err)
		return
	}

	if len(alerts) == 0 {
		return
	}

	metrics.SchedulerStaleAlertsSwept.Set(int64(len(alerts)))
	logger.Info("Scheduler stale sweep found uninvestigated alerts", "component", "scheduler", "count", len(alerts), "threshold", s.staleThreshold)

	groups := s.groupAlertsByCorrelationKey(alerts)
	created := 0

	for key, group := range groups {
		if s.checkStaleCooldown(ctx, key) {
			continue
		}

		existing, err := s.alertInvestigationStore.GetActiveAlertInvestigationByCorrelationKey(ctx, key)
		if err != nil {
			logger.Warn("Scheduler stale sweep failed to check active investigation for key", "component", "scheduler", "correlation_key", key, "error", err)
			continue
		}

		correlated := s.alertsToCorrelated(group)

		if existing != nil {
			if err := s.alertInvestigationStore.AppendAlertsToAlertInvestigation(ctx, existing.AlertInvestigationID, correlated); err != nil {
				logger.Warn("Scheduler stale sweep failed to append alerts to existing investigation", "component", "scheduler", "alert_count", len(correlated), "alert_investigation_id", existing.AlertInvestigationID, "error", err)
				continue
			}
			logger.Info("Scheduler stale sweep appended alerts to existing investigation", "component", "scheduler", "alert_count", len(correlated), "alert_investigation_id", existing.AlertInvestigationID, "correlation_key", key)
			s.setStaleCooldown(ctx, key, existing.AlertInvestigationID)
			created++
			continue
		}

		investigationID := uuid.Must(uuid.NewV7()).String()

		severity := rabbitmq.DetermineAlertSeverity(correlated)
		msg := rabbitmq.InvestigateMessage{
			InvestigationID:   investigationID,
			InvestigationKind: rabbitmq.InvestigationKindAlert,
			Alerts:            correlated,
			Severity:          severity,
			CorrelationKey:    key,
			RetryCount:        0,
			TraceID:           fmt.Sprintf("stale-sweep:%s", key),
			DedupeKey:         fmt.Sprintf("stale:%s:%d", key, time.Now().UnixNano()),
		}

		if err := s.stalePublisher.PublishInvestigation(ctx, msg); err != nil {
			logger.Error("Scheduler stale sweep failed to publish investigation for key", "component", "scheduler", "alert_investigation_id", investigationID, "correlation_key", key, "error", err)
			continue
		}

		s.setStaleCooldown(ctx, key, investigationID)
		created++
		logger.Info("Scheduler stale sweep published investigation", "component", "scheduler", "alert_investigation_id", investigationID, "alert_count", len(correlated), "severity", severity, "correlation_key", key)
	}

	metrics.SchedulerStaleInvestigationsMade.Add(int64(created))
	if created > 0 {
		s.NotifyPending()
	}
}

// runSLASweep is the long-running goroutine that periodically publishes an
// SLA sweep request tick to RabbitMQ so the SLAWorker can detect breaches.
// It exists because nothing else produces SLASweepMessage (the
// decision assigned publication to the scheduler leader rather than an
// external cron, whose silence was the original failure mode).
func (s *InvestigationScheduler) runSLASweep() {
	defer s.wg.Done()
	interval := s.slaSweepInterval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "sla_sweep", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.slaSweepTick(context.Background())
			}()
		}
	}
}

// slaSweepTick runs one pass of the SLA sweep publisher. Only the leader
// publishes ticks; non-leader replicas skip silently. Publish failures log a
// warning and never stop the loop — the next interval retries.
func (s *InvestigationScheduler) slaSweepTick(ctx context.Context) {
	if !s.acquireLeadership(ctx) {
		return
	}
	if err := s.slaSweepPublisher.PublishSLASweep(ctx, rabbitmq.SLASweepMessage{}); err != nil {
		logger.Warn("SLA sweep failed to publish request tick", "component", "scheduler", "error", err)
		return
	}
	logger.Info("SLA sweep published request tick", "component", "scheduler", "interval", s.slaSweepInterval)
}

func (s *InvestigationScheduler) runHandoffTick() {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if s.handoffSkipOnce {
				s.handoffSkipOnce = false
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "handoff", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.handoffTick(context.Background())
			}()
		}
	}
}

func (s *InvestigationScheduler) handoffTick(ctx context.Context) {
	if s.leader != nil && !s.leader.IsLeader() {
		return
	}

	changed, records, err := s.handoffDetector.Tick(ctx, time.Now())
	if err != nil {
		logger.Warn("Handoff detection tick failed", "component", "scheduler", "error", err)
		return
	}
	for _, sched := range changed {
		record := records[sched.ID.String()]
		if record == nil {
			continue
		}
		// The schedule display name is derived dynamically from its team.
		displayName := "On-Call"
		if sched.TeamID != nil && s.teamStore != nil {
			if name, err := s.teamStore.GetTeamName(ctx, *sched.TeamID); err == nil && name != "" {
				displayName = name
			}
		}
		if record.IncomingUserID != nil {
			if err := s.notifyPublisher.PublishNotificationDispatch(ctx, rabbitmq.NotificationDispatchMessage{
				UserID:           record.IncomingUserID.String(),
				NotificationType: "oncall_handoff",
				Title:            fmt.Sprintf("You are now on call for %s", displayName),
				Message:          fmt.Sprintf("Your on-call shift has started for %s. Review handoff notes.", displayName),
				ResourceType:     "handoff",
				ResourceID:       record.ID.String(),
			}); err != nil {
				logger.WarnCtx(ctx, "Handoff sweep: failed to publish incoming handoff notification", "component", "scheduler-sweep", "schedule_id", sched.ID, "error", err)
			}
		}
		if record.OutgoingUserID != nil {
			if err := s.notifyPublisher.PublishNotificationDispatch(ctx, rabbitmq.NotificationDispatchMessage{
				UserID:           record.OutgoingUserID.String(),
				NotificationType: "oncall_handoff",
				Title:            fmt.Sprintf("Your on-call shift for %s has ended", displayName),
				Message:          fmt.Sprintf("Your shift for %s has ended. Please write handoff notes.", displayName),
				ResourceType:     "handoff",
				ResourceID:       record.ID.String(),
			}); err != nil {
				logger.WarnCtx(ctx, "Handoff sweep: failed to publish outgoing handoff notification", "component", "scheduler-sweep", "schedule_id", sched.ID, "error", err)
			}
		}
	}
}

// runPrune periodically deletes alerts older than the retention cutoff
// (DATA_RETENTION_DAYS, 0 = keep forever). Whether the underlying store
// filters by status is implementation-defined.
func (s *InvestigationScheduler) runPrune() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "prune", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.pruneTick(context.Background())
			}()
		}
	}
}

func (s *InvestigationScheduler) pruneTick(ctx context.Context) {
	if !s.acquireLeadership(ctx) {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -s.dataRetentionDays)
	n, err := s.alertStore.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		logger.Error("Data retention prune failed to delete old alerts", "component", "scheduler", "error", err)
		return
	}
	if n > 0 {
		logger.Info("Data retention prune deleted resolved alerts", "component", "scheduler", "count", n, "cutoff", cutoff.Format(time.RFC3339))
	}
}

func (s *InvestigationScheduler) runSummarySweep() {
	defer s.wg.Done()
	interval := s.summaryDefaultInterval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "summary_sweep", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.summarySweepTick()
			}()
		}
	}
}

func (s *InvestigationScheduler) runIncidentSweep() {
	defer s.wg.Done()
	interval := s.staleInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "incident_sweep", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.incidentSweepTick(context.Background())
			}()
		}
	}
}

func (s *InvestigationScheduler) incidentSweepTick(ctx context.Context) {
	if !s.acquireLeadership(ctx) {
		return
	}

	metrics.SchedulerIncidentSweepTickTotal.Add(1)

	if s.incidentStore == nil || s.incidentInvestigationStore == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	incidents, err := s.incidentStore.ListActiveIncidents(ctx)
	if err != nil {
		logger.Error("Incident sweep failed to list active incidents", "component", "scheduler", "error", err)
		return
	}

	created := 0
	for _, inc := range incidents {
		hasInvestigation, err := s.incidentHasOpenInvestigation(ctx, inc.IncidentNumber)
		if err != nil {
			logger.Warn("Incident sweep failed to check incident investigations", "component", "scheduler", "incident_number", inc.IncidentNumber, "error", err)
			continue
		}
		if hasInvestigation {
			continue
		}

		inv, err := s.incidentInvestigationStore.CreateIncidentInvestigation(ctx, store.IncidentInvestigationRecord{
			IncidentNumber: inc.IncidentNumber,
			Status:         "pending",
		})
		if err != nil {
			logger.Error("Incident sweep failed to create investigation", "component", "scheduler", "incident_number", inc.IncidentNumber, "error", err)
			continue
		}
		if inv == nil {
			logger.Warn("Incident sweep create investigation returned nil", "component", "scheduler", "incident_number", inc.IncidentNumber)
			continue
		}

		if s.ssePublisher != nil {
			s.ssePublisher.Publish(sse.Event{
				Type: "investigation_created",
				Data: map[string]any{
					"investigation_id": inv.IncidentInvestigationID,
					"incident_number":  inc.IncidentNumber,
					"status":           inv.Status,
				},
			})
			s.ssePublisher.Publish(sse.Event{
				Type: "incident_updated",
				Data: map[string]string{"incident_number": strconv.FormatInt(inc.IncidentNumber, 10)},
			})
		}
		created++
		logger.Info("Incident sweep created investigation", "component", "scheduler", "incident_number", inc.IncidentNumber, "investigation_id", inv.IncidentInvestigationID)
	}

	if created > 0 {
		s.NotifyPending()
	}
}

func (s *InvestigationScheduler) incidentHasOpenInvestigation(ctx context.Context, incidentNumber int64) (bool, error) {
	invs, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
	if err != nil {
		return false, err
	}
	for _, inv := range invs {
		switch inv.Status {
		case store.IncidentInvestigationStatusPending,
			store.IncidentInvestigationStatusAssigned,
			store.IncidentInvestigationStatusInvestigating,
			store.IncidentInvestigationStatusPaused:
			return true, nil
		}
	}
	return false, nil
}

func (s *InvestigationScheduler) summarySweepTick() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if s.leader != nil && !s.leader.IsLeader() {
		return
	}
	if s.incidentStore == nil || s.incidentChannelMgr == nil || !s.incidentChannelMgr.IsSupported() {
		return
	}

	metrics.SummarySweepTotal.Add(1)

	incidents, err := s.incidentStore.ListActiveSummarizableIncidents(ctx)
	if err != nil {
		logger.Error("Summary sweep list incidents failed", "component", "scheduler", "error", err)
		return
	}

	for _, inc := range incidents {
		interval := s.effectiveSummaryInterval(inc.Severity)
		incidentID := strconv.FormatInt(inc.IncidentNumber, 10)

		lastKey := "alga:summary:last:" + incidentID
		pendingKey := "alga:summary:pending:" + incidentID

		if s.valkeyClient != nil {
			n, err := s.valkeyClient.Do(ctx, s.valkeyClient.Builder().Exists().Key(lastKey).Build()).AsInt64()
			if err == nil && n > 0 {
				metrics.SummarySkippedTotal.Add(1)
				continue
			}
			n, err = s.valkeyClient.Do(ctx, s.valkeyClient.Builder().Exists().Key(pendingKey).Build()).AsInt64()
			if err == nil && n > 0 {
				metrics.SummarySkippedTotal.Add(1)
				continue
			}
		}

		agentIDHex, agentName := s.findIncidentAgent(ctx, inc.IncidentNumber)
		if agentIDHex == "" {
			metrics.SummarySkippedTotal.Add(1)
			continue
		}

		if !s.isAgentOnline(ctx, agentIDHex) {
			metrics.SummarySkippedTotal.Add(1)
			continue
		}

		durationMinutes := int(time.Since(inc.CreatedAt).Minutes())
		evt := sse.Event{
			Type: "summarize_incident",
			Data: map[string]any{
				"incident_number": inc.IncidentNumber,
				"chat_id":         "incident_coord_" + incidentID,
				"incident": map[string]any{
					"title":                inc.Title,
					"severity":             inc.Severity,
					"status":               inc.Status,
					"duration_minutes":     durationMinutes,
					"timeline_entry_count": len(inc.Timeline),
				},
			},
		}

		if err := s.resolver.ForwardEventToAgent(agentIDHex, evt); err != nil {
			logger.Warn("Summary sweep failed to send summarize_incident to agent", "component", "scheduler", "agent_id", agentIDHex, "incident_number", inc.IncidentNumber, "error", err)
			continue
		}

		if s.valkeyClient != nil {
			ttlSec := int64((interval * 2).Seconds())
			_ = s.valkeyClient.Do(ctx, s.valkeyClient.Builder().Set().Key(pendingKey).Value("1").ExSeconds(ttlSec).Build()).Error()
		}

		metrics.SummaryDispatchedTotal.Add(1)
		logger.Info("Summary sweep dispatched summarize_incident", "component", "scheduler", "incident_number", inc.IncidentNumber, "agent_name", agentName)
	}
}

func (s *InvestigationScheduler) findIncidentAgent(ctx context.Context, incidentNumber int64) (agentIDHex string, agentName string) {
	if s.incidentInvestigationStore == nil {
		return "", ""
	}
	invs, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
	if err != nil {
		return "", ""
	}
	for _, inv := range invs {
		if inv.Status == "investigating" && inv.AgentID != "" {
			return inv.AgentID, inv.AgentName
		}
	}
	for _, inv := range invs {
		if inv.AgentID != "" && !store.IsTerminalInvestigationStatus(inv.Status) {
			return inv.AgentID, inv.AgentName
		}
	}
	return "", ""
}

func (s *InvestigationScheduler) isAgentOnline(ctx context.Context, agentIDHex string) bool {
	if s.presence != nil && s.presence.Available() {
		if s.presence.IsAgentOnline(ctx, agentIDHex) {
			return true
		}
	}
	return s.resolver.AgentOnline(agentIDHex)
}

// groupAlertsByCorrelationKey groups alerts by their correlation key, using
// the same key derivation logic as the correlator.
func (s *InvestigationScheduler) groupAlertsByCorrelationKey(alerts []store.AlertRecord) map[string][]store.AlertRecord {
	groups := make(map[string][]store.AlertRecord, len(alerts))
	for _, a := range alerts {
		key, _ := correlator.CorrelationKey(a.Labels)
		groups[key] = append(groups[key], a)
	}
	return groups
}

// alertsToCorrelated converts AlertRecords to CorrelatedAlert snapshots for
// the InvestigateMessage payload.
func (s *InvestigationScheduler) alertsToCorrelated(alerts []store.AlertRecord) []rabbitmq.CorrelatedAlert {
	out := make([]rabbitmq.CorrelatedAlert, 0, len(alerts))
	for _, a := range alerts {
		ca := rabbitmq.CorrelatedAlert{
			Fingerprint:  a.Fingerprint,
			AlertNumber:  a.AlertNumber,
			Labels:       a.Labels,
			Annotations:  a.Annotations,
			Status:       a.Status,
			GeneratorURL: a.GeneratorURL,
		}
		if !a.StartsAt.IsZero() {
			ca.StartsAt = a.StartsAt.UTC().Format(time.RFC3339)
		}
		out = append(out, ca)
	}
	return out
}

// checkStaleCooldown returns true when a Valkey cooldown entry exists for the
// given correlation key, meaning a recent investigation was already created
// for this key (by the normal correlator or a prior stale sweep).
func (s *InvestigationScheduler) checkStaleCooldown(ctx context.Context, key string) bool {
	if s.valkeyClient == nil {
		return false
	}
	cd, err := s.valkeyClient.Do(ctx, s.valkeyClient.Builder().Get().Key("alga:cooldown:"+key).Build()).AsBytes()
	if err != nil {
		return false
	}
	return len(cd) > 0
}

// setStaleCooldown writes a cooldown entry to Valkey so the next stale sweep
// (or normal correlator pass) does not create a duplicate investigation for
// the same correlation key.
func (s *InvestigationScheduler) setStaleCooldown(ctx context.Context, key, investigationID string) {
	if s.valkeyClient == nil {
		return
	}
	entry := struct {
		InvestigationID string `json:"investigation_id"`
	}{InvestigationID: investigationID}
	data, _ := json.Marshal(entry)
	ttlSec := int64((30 * time.Minute).Seconds())
	if err := s.valkeyClient.Do(ctx, s.valkeyClient.Builder().Set().
		Key("alga:cooldown:"+key).Value(string(data)).ExSeconds(ttlSec).Build()).Error(); err != nil {
		logger.Warn("Scheduler stale sweep failed to set cooldown for key", "component", "scheduler", "correlation_key", key, "error", err)
	}
}

func (s *InvestigationScheduler) nudgeStalled(ctx context.Context) {
	if s.resolver == nil {
		return
	}

	assignedNudge := s.investigationTimeout / 2
	if assignedNudge < time.Minute {
		assignedNudge = time.Minute
	}
	investigatingNudge := s.investigationTimeout * 3 / 4
	if investigatingNudge < time.Minute {
		investigatingNudge = time.Minute
	}

	s.nudgeAssigned(ctx, assignedNudge)
	s.nudgeInvestigating(ctx, investigatingNudge)
}

func (s *InvestigationScheduler) nudgeByStatus(ctx context.Context, listFn func(ctx context.Context, threshold time.Duration) ([]store.AlertInvestigationRecord, error), label string, threshold time.Duration) {
	stalled, err := listFn(ctx, threshold)
	if err != nil {
		logger.Error("Scheduler failed to list stalled investigations for nudge", "component", "scheduler", "status", label, "error", err)
		return
	}

	for _, inv := range stalled {
		if s.alertStore != nil && len(inv.Alerts) > 0 && !hasCurrentActiveAlert(s.alertStore, inv.Alerts) {
			continue
		}

		if _, loaded := s.nudged.LoadOrStore(inv.AlertInvestigationID, struct{}{}); loaded {
			continue
		}

		if inv.AgentID == "" || !s.resolver.AgentOnline(inv.AgentID) {
			continue
		}

		var elapsed time.Duration
		if inv.StartedAt != nil {
			elapsed = time.Since(*inv.StartedAt).Truncate(time.Second)
		}
		input := prompt.FromAlertInvestigationRecord(&inv)
		s.enrichWithOpsTeam(ctx, &input)
		p := s.buildDispatchPrompt(ctx, input)
		sc := s.buildDispatchSystemContext(input)
		if err := s.resolver.ForwardDispatchToAgent(inv.AgentID, inv.AlertInvestigationID, "system", "System", p, sc); err != nil {
			logger.Warn("Scheduler failed to re-dispatch investigation to agent", "component", "scheduler", "status", label, "alert_investigation_id", inv.AlertInvestigationID, "agent_name", inv.AgentName, "error", err)
			s.nudged.Delete(inv.AlertInvestigationID)
			continue
		}

		metrics.SchedulerNudgeTotal.Add(1)
		logger.Info("Scheduler re-dispatched investigation to agent", "component", "scheduler", "status", label, "alert_investigation_id", inv.AlertInvestigationID, "agent_name", inv.AgentName, "elapsed", elapsed)
	}
}

func (s *InvestigationScheduler) buildDispatchPrompt(ctx context.Context, input prompt.DispatchInput) string {
	p := prompt.BuildDispatchPromptWithKnowledge(ctx, input, s.knowledge)
	if s.playbookEnricher != nil && len(input.Alerts) > 0 {
		if extra := s.playbookEnricher.Enrich(ctx, input.Alerts[0].Labels); extra != "" {
			p += extra
		}
	}
	if input.IncidentScope && input.IncidentID != "" && s.incidentStore != nil {
		if incCtx := s.buildIncidentContext(input.IncidentID); incCtx != "" {
			p = incCtx + p
		}
	}
	return p
}

func (s *InvestigationScheduler) buildDispatchSystemContext(input prompt.DispatchInput) string {
	return prompt.BuildDispatchSystemContext(input)
}

func (s *InvestigationScheduler) nudgeAssigned(ctx context.Context, threshold time.Duration) {
	s.nudgeByStatus(ctx, s.alertInvestigationStore.ListStalledAssignedAlertInvestigations, "assigned", threshold)
}

func (s *InvestigationScheduler) nudgeInvestigating(ctx context.Context, threshold time.Duration) {
	s.nudgeByStatus(ctx, s.alertInvestigationStore.ListStalledInvestigatingAlertInvestigations, "investigating", threshold)
}

// computeSpecificity scores label selectors so more specific selectors win.
// We weight the operator type (exact > prefix/contains > wildcard > regex)
// because exact matches express stronger operator intent than fuzzy ones.
func computeSpecificity(conditions []config.RouteCondition) int {
	if len(conditions) == 0 {
		return 0
	}
	score := len(conditions) * 10
	for _, c := range conditions {
		score += operatorWeight(c.Operator)
	}
	return score
}

func operatorWeight(op string) int {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "exact":
		return 5
	case "contains", "prefix", "suffix":
		return 3
	case "wildcard":
		return 2
	case "regex", "exists", "not_exists":
		return 1
	default:
		return 0
	}
}

func extractLabels(inv store.AlertInvestigationRecord) map[string]string {
	if len(inv.Alerts) > 0 {
		return inv.Alerts[0].Labels
	}
	return map[string]string{}
}

var severityWeight = map[string]float64{
	"critical": 1000,
	"high":     500,
	"warning":  100,
	"info":     10,
}

func computePriority(inv store.AlertInvestigationRecord) float64 {
	severity := rabbitmq.DetermineAlertSeverity(inv.Alerts)
	w, ok := severityWeight[severity]
	if !ok {
		w = 10
	}
	ageMinutes := time.Since(inv.CreatedAt).Minutes()
	return w + ageMinutes
}

func (s *InvestigationScheduler) enrichWithOpsTeam(ctx context.Context, input *prompt.DispatchInput) {
	if s.teamStore == nil {
		return
	}
	opsTeam, err := s.teamStore.GetTeamByName(ctx, "ops-team")
	if err != nil || opsTeam == nil {
		return
	}
	input.AdminTeamID = opsTeam.ID.String()
	input.AdminTeamName = opsTeam.Name
}

// buildIncidentContext fetches the incident record and returns a prompt
// prefix with incident description, severity, status, and timeline summary.
// Returns empty string when the incident cannot be loaded.
func (s *InvestigationScheduler) buildIncidentContext(incidentID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	incidentNumber, parseErr := strconv.ParseInt(incidentID, 10, 64)
	if parseErr != nil {
		return ""
	}
	inc, err := s.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		logger.Warn("Scheduler failed to load incident context for investigation prompt", "component", "scheduler", "incident_id", incidentID, "error", err)
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "This investigation is scoped to Incident **%d** (#%d).\n\n", inc.IncidentNumber, inc.IncidentNumber)
	fmt.Fprintf(&b, "**Incident Title:** %s\n", inc.Title)
	fmt.Fprintf(&b, "**Incident Status:** %s\n", inc.Status)
	fmt.Fprintf(&b, "**Incident Severity:** %s\n", inc.Severity)
	fmt.Fprintf(&b, "**Impact Level:** %s\n", inc.ImpactLevel)
	if inc.Description != "" {
		fmt.Fprintf(&b, "**Incident Description:** %s\n", inc.Description)
	}
	if s.alertStore != nil {
		fingerprints, alertsErr := s.alertStore.GetAlertsByIncident(ctx, inc.IncidentNumber)
		if alertsErr != nil {
			logger.Warn("Scheduler failed to load linked alerts for incident prompt", "component", "scheduler", "incident_number", inc.IncidentNumber, "error", alertsErr)
		}
		fmt.Fprintf(&b, "**Linked Alerts:** %d\n", len(fingerprints))
		for _, fp := range fingerprints {
			rec, getByFpErr := s.alertStore.GetByFingerprint(fp)
			if getByFpErr != nil || rec == nil {
				fmt.Fprintf(&b, "- %s\n", fp)
				continue
			}
			// Skip soft-deleted alerts: the Linked Alerts card on the incident
			// detail page surfaces tombstones to operators, but the agent
			// investigation prompt should only act on live alerts.
			if rec.DeletedAt != nil {
				continue
			}
			name := rec.Labels["alertname"]
			if name == "" {
				name = fp
			}
			if rec.AlertNumber > 0 {
				fmt.Fprintf(&b, "- **#%d** %s — status: %s\n", rec.AlertNumber, name, rec.Status)
			} else {
				fmt.Fprintf(&b, "- %s — status: %s\n", name, rec.Status)
			}
		}
	}
	if len(inc.Timeline) > 0 {
		fmt.Fprintf(&b, "\n**Incident Timeline Summary:**\n")
		for _, entry := range inc.Timeline {
			fmt.Fprintf(&b, "- %s (%s): %s\n", entry.CreatedAt.Format(time.RFC3339), entry.EventType, entry.Message)
		}
	}
	if incRoles, err := s.icsRoleStore.GetActiveRoles(ctx, incidentNumber); err == nil && len(incRoles) > 0 {
		fmt.Fprintf(&b, "\n**Incident Roles:** Commander directs and documents, Communicator handles updates, Responders investigate and mitigate.\n")
		for _, role := range incRoles {
			name := role.UserName
			if name == "" {
				name = role.AgentName
			}
			label := role.RoleType
			if l := ics.RoleLabel(ics.RoleType(role.RoleType)); l != role.RoleType {
				label = l
			}
			fmt.Fprintf(&b, "- %s: %s\n", label, name)
		}
	}
	fmt.Fprintf(&b, "\n**Role expectations:**\n")
	fmt.Fprintf(&b, "- %s: %s.\n", ics.RoleLabel(ics.RoleIncidentCommander), ics.RoleResponsibilityPrompt(ics.RoleIncidentCommander))
	fmt.Fprintf(&b, "- %s: %s.\n", ics.RoleLabel(ics.RoleCommunicationsLead), ics.RoleResponsibilityPrompt(ics.RoleCommunicationsLead))
	fmt.Fprintf(&b, "- %s: %s.\n", ics.RoleLabel(ics.RoleResponder), ics.RoleResponsibilityPrompt(ics.RoleResponder))
	fmt.Fprintf(&b, "\n**Coordination & mentions:** Reference the incident and alerts by NUMBER only (e.g. \"Incident #%d\", \"Alert #42\") — never mention, echo, or surface investigation IDs, incident IDs, or UUIDs; they are not user-facing or linkable. To address a teammate in the coordination thread, reuse the exact `[@Name](agent:UUID)` mention form you received from them (a UUID looks like `123e4567-e89b-12d3-a456-426614174000` — never wrap it in angle brackets or quotes); do not invent role abbreviations such as @ic, @comms, or @cmd — they are not valid mentions and will not resolve. Only @mention a teammate when you need them to take an action; never @mention to thank, acknowledge, agree, sign off, or say goodbye — a no-mention reply (or no reply at all) is preferred. Do NOT @mention another agent or teammate unless absolutely necessary to request an action or handoff. Do NOT mention them in status updates, replies, findings, or handoffs. Mentioning other agents activates their models, which causes unnecessary message loops. Do not respond to a teammate's message that is purely a thank-you, acknowledgement, agreement, greeting, or sign-off. After this incident reaches `resolved` or `closed`, do not post further coordination messages unless a human operator asks you to.\n", inc.IncidentNumber)
	b.WriteString("\n---\n\n")
	return b.String()
}

// matchConditions evaluates an agent's label-selector conditions against
// alert labels. All conditions must pass (AND semantics).
func (s *InvestigationScheduler) matchConditions(conditions []config.RouteCondition, labels map[string]string) bool {
	if len(conditions) == 0 {
		return false
	}
	for _, c := range conditions {
		if !s.matchCondition(c, labels) {
			return false
		}
	}
	return true
}

func (s *InvestigationScheduler) matchCondition(c config.RouteCondition, labels map[string]string) bool {
	field := strings.TrimSpace(c.Field)
	actual, exists := labels[field]
	op := strings.ToLower(strings.TrimSpace(c.Operator))
	value := c.Value

	switch op {
	case "exists":
		return exists && strings.TrimSpace(actual) != ""
	case "not_exists":
		return !exists || strings.TrimSpace(actual) == ""
	case "contains":
		return strings.Contains(actual, value)
	case "prefix":
		return strings.HasPrefix(actual, value)
	case "suffix":
		return strings.HasSuffix(actual, value)
	case "wildcard":
		return wildcardMatch(value, actual)
	case "regex":
		if len(value) > 256 {
			return false
		}
		re, err := matching.GetCompiledRegex(value)
		if err != nil {
			return false
		}
		return re.MatchString(actual)
	case "exact":
		fallthrough
	default:
		return actual == value
	}
}

func wildcardMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		prefix, suffix := parts[0], parts[1]
		return strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix) && len(s) >= len(prefix)+len(suffix)
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	return strings.HasSuffix(s, parts[len(parts)-1])
}

// buildDiscriminators converts an alert's label map into a bounded discriminator
// map for the Valkey active-investigation index. The 8-key cap bounds the
// per-investigation SADD cost for noisy alerts. Returns nil when labels is
// empty so callers can pass the result straight to the Valkey call.
func buildDiscriminators(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	const maxDiscriminators = 8
	out := make(map[string]string, min(len(labels), maxDiscriminators))
	for k, v := range labels {
		out[k] = v
		if len(out) >= maxDiscriminators {
			break
		}
	}
	return out
}
