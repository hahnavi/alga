// scheduler_coordination_tasks.go contains the CoordinationTask dispatch pass
// and its stall sweep: affinity-aware, role-targeted assignment of pending
// CoordinationTasks to eligible online agents, plus the periodic sweep that
// reverts timed-out tasks (after K dispatch attempts) and dead-letters them.
//
// The dispatch pass mirrors scheduleIncidentInvestigations; the sweep mirrors
// runStaleSweep/staleSweepTick. Both are gated by the leader lease when HA is
// configured.
package worker

import (
	"context"
	"errors"
	"runtime/debug"
	"strconv"
	"time"

	"alga/capability"
	"alga/ics"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

// coordinationTaskMaxFailures is the number of failed dispatch/sweep attempts
// (tracked via dispatch_attempts) before an overdue task is dead-lettered.
const coordinationTaskMaxFailures = 3

// scheduleCoordinationTasks runs one Filter → Affinity → Claim → Dispatch pass
// over pending CoordinationTasks. It is invoked from the main tick alongside
// scheduleIncidentInvestigations. Tasks are role-targeted: a commander task
// goes only to agents with the "command" capability, a communicator task to
// "communicate", and a responder task to "investigate". When a task targets a
// specific agent (AssigneeAgentID set), that agent is preferred when eligible.
// Commanders are excluded from investigate-kind tasks (they are pure
// orchestrators).
func (s *InvestigationScheduler) scheduleCoordinationTasks(ctx context.Context) {
	if s.coordinationTaskStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	agents, err := s.agentTokenStore.ListActiveAgents()
	if err != nil {
		logger.Error("Scheduler failed to list active agents for coordination tasks", "component", "scheduler", "error", err)
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

	// ListPendingTasks is role-scoped at the store layer; gather pending tasks
	// across all three coordination roles.
	roles := []string{
		store.CoordinationTaskRoleCommander,
		store.CoordinationTaskRoleCommunicator,
		store.CoordinationTaskRoleResponder,
	}
	var pending []store.CoordinationTaskRecord
	for _, role := range roles {
		tasks, listErr := s.coordinationTaskStore.ListPendingTasks(ctx, role, 100)
		if listErr != nil {
			logger.Error("Scheduler failed to list pending coordination tasks", "component", "scheduler", "role", role, "error", listErr)
			continue
		}
		pending = append(pending, tasks...)
	}
	if len(pending) == 0 {
		return
	}

	for i := range pending {
		task := pending[i]
		if task.Status != store.CoordinationTaskStatusPending {
			continue
		}

		requiredCap := capabilityForRole(task.AssigneeRole)
		if requiredCap == "" {
			// Unknown role — cannot route; bump so the sweep eventually fails it.
			if err := s.coordinationTaskStore.BumpDispatchAttempts(ctx, task.ID); err != nil {
				logger.Warn("Scheduler failed to bump unknown-role coordination task", "component", "scheduler", "task_id", task.ID, "role", task.AssigneeRole, "error", err)
			}
			continue
		}

		// Build the eligible candidate set: online + capability + capacity, with
		// commanders excluded from investigate-kind tasks.
		candidates := make(map[string]*store.AgentTokenRecord, len(online))
		for id, a := range online {
			if !capability.Has(a.Capabilities, requiredCap) {
				continue
			}
			if capacityCache[id] >= s.maxConcurrent {
				continue
			}
			if task.Kind == store.CoordinationTaskKindInvestigate {
				if role := s.resolveIncidentRole(ctx, task.IncidentNumber, id); role == string(ics.RoleIncidentCommander) {
					continue
				}
			}
			candidates[id] = a
		}
		if len(candidates) == 0 {
			if err := s.coordinationTaskStore.BumpDispatchAttempts(ctx, task.ID); err != nil {
				logger.Warn("Scheduler failed to bump no-candidate coordination task", "component", "scheduler", "task_id", task.ID, "error", err)
			}
			continue
		}

		// Affinity: when an assignee is named, prefer it if still eligible.
		var chosen *store.AgentTokenRecord
		if task.AssigneeAgentID != "" {
			if a, ok := candidates[task.AssigneeAgentID]; ok {
				chosen = a
			}
		}
		if chosen == nil {
			chosen = pickLeastLoaded(candidates, capacityCache)
		}
		if chosen == nil {
			if err := s.coordinationTaskStore.BumpDispatchAttempts(ctx, task.ID); err != nil {
				logger.Warn("Scheduler failed to bump no-candidate coordination task", "component", "scheduler", "task_id", task.ID, "error", err)
			}
			continue
		}

		agentIDHex := chosen.ID.String()
		claimed, claimErr := s.coordinationTaskStore.ClaimTask(ctx, task.ID, task.AssigneeRole, agentIDHex, chosen.Name)
		if claimErr != nil {
			if errors.Is(claimErr, store.ErrCoordinationTaskStatusConflict) {
				// Someone else claimed it concurrently; skip silently.
				continue
			}
			logger.Error("Scheduler failed to claim coordination task", "component", "scheduler", "task_id", task.ID, "agent_name", chosen.Name, "error", claimErr)
			continue
		}

		if s.dispatchCoordinationTask(ctx, claimed, chosen) {
			capacityCache[agentIDHex]++
		}
	}
}

// dispatchCoordinationTask forwards a claimed coordination task to the chosen
// agent and transitions it to in_progress. On forward failure the task is
// reverted to pending (via BumpDispatchAttempts, which also increments the
// failure counter) and the agent's health tracker records the failure. Returns
// true on a successful dispatch.
func (s *InvestigationScheduler) dispatchCoordinationTask(ctx context.Context, task *store.CoordinationTaskRecord, agent *store.AgentTokenRecord) bool {
	agentIDHex := agent.ID.String()

	chatID := "incident_coord_" + strconv.FormatInt(task.IncidentNumber, 10)
	incidentID := ""
	if task.IncidentID != nil {
		incidentID = task.IncidentID.String()
	}

	evt := sse.Event{
		Type: "coordination_task_dispatched",
		Data: map[string]any{
			"chat_id":         chatID,
			"task_id":         task.ID.String(),
			"kind":            task.Kind,
			"assignee_role":   task.AssigneeRole,
			"goal":            task.Goal,
			"text":            task.Goal,
			"trigger":         "dispatch",
			"incident_number": task.IncidentNumber,
			"incident_id":     incidentID,
			"agent_id":        agentIDHex,
			"agent_name":      agent.Name,
		},
	}

	if err := s.resolver.ForwardEventToAgent(agentIDHex, evt); err != nil {
		logger.Error("Scheduler failed to forward coordination task to agent", "component", "scheduler", "task_id", task.ID, "agent_name", agent.Name, "error", err)
		// Revert to pending so the task can be redispatched; the bump also
		// increments the failure counter so the sweep can dead-letter it after
		// K repeated failures.
		if revertErr := s.coordinationTaskStore.BumpDispatchAttempts(ctx, task.ID); revertErr != nil {
			logger.Warn("Scheduler failed to revert coordination task after forward failure", "component", "scheduler", "task_id", task.ID, "error", revertErr)
		}
		s.healthTracker.RecordFailure(agentIDHex)
		return false
	}

	// Best-effort transition to in_progress; a log-only failure does not undo
	// the forward — the sweep will reconcile a stuck "assigned" task.
	if err := s.coordinationTaskStore.MarkInProgress(ctx, task.ID); err != nil {
		logger.Warn("Scheduler failed to mark coordination task in_progress", "component", "scheduler", "task_id", task.ID, "error", err)
	}

	s.healthTracker.RecordSuccess(agentIDHex)
	logger.Info("Scheduler assigned coordination task to agent", "component", "scheduler", "task_id", task.ID, "kind", task.Kind, "assignee_role", task.AssigneeRole, "agent_name", agent.Name, "agent_id", agentIDHex)

	if s.ssePublisher != nil {
		s.ssePublisher.Publish(evt)
	}
	return true
}

// runCoordinationTaskSweep is the long-running goroutine that periodically
// reverts timed-out coordination tasks and dead-letters them after
// coordinationTaskMaxFailures attempts. It runs on a slower cadence than the
// main scheduling tick and is gated by the same leader lease when HA is
// configured.
func (s *InvestigationScheduler) runCoordinationTaskSweep() {
	defer s.wg.Done()
	interval := s.coordinationTaskSweepInterval
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
						logger.Error("scheduler tick panicked", "component", "scheduler", "tick", "coordination_task_sweep", "panic", r, "stack", string(debug.Stack()))
					}
				}()
				s.coordinationTaskSweepTick(context.Background())
			}()
		}
	}
}

// coordinationTaskSweepTick runs one pass of the coordination task stall
// sweep. Only the leader runs the sweep; non-leader replicas skip silently.
func (s *InvestigationScheduler) coordinationTaskSweepTick(ctx context.Context) {
	if s.coordinationTaskStore == nil {
		return
	}
	if !s.acquireLeadership(ctx) {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	overdue, err := s.coordinationTaskStore.ListOverdueTasks(ctx, time.Now().UTC(), 100)
	if err != nil {
		logger.Error("Coordination task sweep failed to list overdue tasks", "component", "scheduler", "error", err)
		return
	}
	if len(overdue) == 0 {
		return
	}

	reverted := 0
	failed := 0
	for i := range overdue {
		task := overdue[i]

		if task.DispatchAttempts >= coordinationTaskMaxFailures {
			if failErr := s.coordinationTaskStore.FailTask(ctx, task.ID, "timed out: no agent claimed or completed within due window"); failErr != nil {
				logger.Error("Coordination task sweep failed to dead-letter task", "component", "scheduler", "task_id", task.ID, "error", failErr)
				continue
			}
			failed++
			logger.Warn("Coordination task dead-lettered after exceeding max failures", "component", "scheduler", "task_id", task.ID, "kind", task.Kind, "assignee_role", task.AssigneeRole, "dispatch_attempts", task.DispatchAttempts)
			if s.ssePublisher != nil {
				s.ssePublisher.Publish(sse.Event{
					Type: "coordination_task_failed",
					Data: map[string]any{
						"task_id":           task.ID.String(),
						"kind":              task.Kind,
						"assignee_role":     task.AssigneeRole,
						"incident_number":   task.IncidentNumber,
						"dispatch_attempts": task.DispatchAttempts,
						"reason":            "timed out: no agent claimed or completed within due window",
					},
				})
			}
			continue
		}

		// Below the failure threshold: revert to pending so it can be
		// redispatched. BumpDispatchAttempts increments the counter and clears
		// the assignee, making the task eligible for the dispatch pass again.
		if revertErr := s.coordinationTaskStore.BumpDispatchAttempts(ctx, task.ID); revertErr != nil {
			logger.Error("Coordination task sweep failed to revert overdue task", "component", "scheduler", "task_id", task.ID, "error", revertErr)
			continue
		}
		reverted++
	}

	if reverted > 0 {
		logger.Info("Coordination task sweep reverted overdue tasks", "component", "scheduler", "count", reverted)
		s.NotifyPending()
	}
	if failed > 0 {
		logger.Warn("Coordination task sweep dead-lettered overdue tasks", "component", "scheduler", "count", failed)
	}
}

// capabilityForRole maps a coordination task's assignee role to the agent
// capability required to execute it. Returns "" for an unrecognized role.
func capabilityForRole(role string) string {
	switch role {
	case store.CoordinationTaskRoleCommander:
		return capability.Command
	case store.CoordinationTaskRoleCommunicator:
		return capability.Communicate
	case store.CoordinationTaskRoleResponder:
		return capability.Investigate
	default:
		return ""
	}
}
