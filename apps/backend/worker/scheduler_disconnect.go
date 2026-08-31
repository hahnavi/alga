// scheduler_disconnect.go contains the agent connect/disconnect handling
// for the investigation scheduler: the disconnect-grace timer, the per-agent
// Valkey lock that fences it, and the cleanup of ICS roles when an agent
// stays away past the grace period.
package worker

import (
	"context"
	"time"

	"github.com/google/uuid"

	"alga/ics"
	"alga/logger"
)

// defaultDisconnectGrace is the default delay before an agent's
// "investigating" work is reset to pending after disconnect. Matches the
// AGENT_DISCONNECT_GRACE config default (45s) so the fallback and the
// configured default agree.
const defaultDisconnectGrace = 45 * time.Second

// disconnectGraceLockTTL is the TTL on the per-agent grace lock. Set
// generously above the grace itself so concurrent retries don't race.
const disconnectGraceLockTTL = 5 * time.Minute

// SetDisconnectGrace overrides the default investigating-reset grace
// period. A zero or negative value reverts to the default.
func (s *InvestigationScheduler) SetDisconnectGrace(d time.Duration) {
	if d <= 0 {
		s.disconnectGrace = defaultDisconnectGrace
		return
	}
	s.disconnectGrace = d
}

func (s *InvestigationScheduler) OnAgentOnline(agentIDHex string) {
	s.NotifyPending()
}

// OnAgentOffline is called when an agent fully disconnects (the event fires on
// every replica, including the one that hosted the sessions). It shortens the
// agent's dispatch leases instead of resetting rows directly:
//
//  1. "assigned" investigations (not yet acked by the agent) lapse
//     immediately, so the lease sweeper requeues them within a tick.
//  2. "investigating" investigations get one disconnect-grace lease window.
//     If the agent reconnects within the grace, its heartbeat renews the
//     lease and the work survives; otherwise the sweeper requeues it.
//
// "paused" rows are deliberately untouched: the agent paused them on purpose.
// The grace timer retained below only tears down the agent's ICS roles after
// the grace, still fenced by a Valkey SET NX lock so only one replica runs it.
func (s *InvestigationScheduler) OnAgentOffline(agentIDHex string) {
	if agentIDHex == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	grace := s.disconnectGrace
	if grace <= 0 {
		grace = defaultDisconnectGrace
	}
	if err := s.alertInvestigationStore.ExpireAlertInvestigationLeasesByAgent(ctx, agentIDHex, grace); err != nil {
		logger.Error("Failed to expire alert investigation leases for agent", "component", "scheduler", "agent_id", agentIDHex, "error", err)
	} else {
		s.NotifyPending()
	}
	if s.incidentInvestigationStore != nil {
		if err := s.incidentInvestigationStore.ExpireIncidentInvestigationLeasesByAgent(ctx, agentIDHex, grace); err != nil {
			logger.Error("Failed to expire incident investigation leases for agent", "component", "scheduler", "agent_id", agentIDHex, "error", err)
		}
	}

	if !s.acquireDisconnectLock(ctx, agentIDHex) {
		return
	}

	// Guard against sync.WaitGroup misuse: Add must not run concurrently with
	// Wait. If Stop has already been invoked, spin down without registering
	// new work so Stop's Wait can return promptly.
	if s.stopped.Load() {
		s.releaseDisconnectLock(ctx, agentIDHex)
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.releaseDisconnectLock(context.Background(), agentIDHex)
		defer func() {
			if r := recover(); r != nil {
				logger.Error("goroutine panic recovered", "panic", r, "location", "scheduler-agent-disconnect-grace", "agent_id", agentIDHex)
			}
		}()
		select {
		case <-s.stopCh:
			return
		case <-time.After(grace):
		}
		if s.agentReturnedOnline(agentIDHex) {
			logger.Info("Agent reconnected within grace; skipping ICS role teardown", "component", "scheduler", "agent_id", agentIDHex)
			return
		}
		if agentUID, parseErr := uuid.Parse(agentIDHex); parseErr == nil {
			s.endAgentRolesOnDisconnect(context.Background(), agentUID)
		}
	}()
}

// acquireDisconnectLock takes a Valkey lock keyed on the agent so that
// only one replica runs the disconnect-grace timer. Falls back to "lock
// held" (true) when Valkey is unavailable; in single-replica deployments
// that's the correct behavior.
func (s *InvestigationScheduler) acquireDisconnectLock(ctx context.Context, agentIDHex string) bool {
	if s.valkeyClient == nil {
		return true
	}
	key := disconnectLockKey(agentIDHex)
	ok, err := s.valkeyClient.SetNX(ctx, key, "1", disconnectGraceLockTTL)
	if err != nil {
		logger.Warn("Disconnect lock SETNX failed; proceeding", "component", "scheduler", "agent_id", agentIDHex, "error", err)
		return true
	}
	return ok
}

func (s *InvestigationScheduler) releaseDisconnectLock(ctx context.Context, agentIDHex string) {
	if s.valkeyClient == nil {
		return
	}
	if err := s.valkeyClient.Del(ctx, disconnectLockKey(agentIDHex)); err != nil {
		logger.Warn("Disconnect lock release failed", "component", "scheduler", "agent_id", agentIDHex, "error", err)
	}
}

// agentReturnedOnline checks whether the agent has reconnected (on any
// replica) since we observed the disconnect. Used at end of grace.
func (s *InvestigationScheduler) agentReturnedOnline(agentIDHex string) bool {
	if s.presence != nil && s.presence.Available() {
		if s.presence.IsAgentOnline(context.Background(), agentIDHex) {
			return true
		}
	}
	if s.resolver != nil && s.resolver.AgentOnline(agentIDHex) {
		return true
	}
	return false
}

func disconnectLockKey(agentIDHex string) string {
	return "alga:agent-disconnect-lock:" + agentIDHex
}

func (s *InvestigationScheduler) endAgentRolesOnDisconnect(ctx context.Context, agentTokenID uuid.UUID) {
	if s.icsRoleStore == nil {
		return
	}
	err := s.icsRoleStore.EndRolesForAgent(ctx, agentTokenID, ics.EndReasonAgentOffline)
	if err != nil {
		logger.Error("failed to end agent ICS roles on disconnect",
			"component", "scheduler", "agent_id", agentTokenID, "error", err)
	}
}
