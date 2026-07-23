package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/capability"
	entschema "alga/ent/schema"
	"alga/ics"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

// validCoordinationTaskKinds is the set of task kinds a commander may dispatch.
// "synthesize" is excluded — it is the kind the synthesize_findings tool itself
// embodies, not something a commander dispatches as a sub-task.
var validCoordinationTaskKinds = map[string]bool{
	store.CoordinationTaskKindInvestigate: true,
	store.CoordinationTaskKindCommunicate: true,
	store.CoordinationTaskKindVerify:      true,
	store.CoordinationTaskKindMitigate:    true,
}

var validCoordinationTaskRoles = map[string]bool{
	store.CoordinationTaskRoleCommander:    true,
	store.CoordinationTaskRoleCommunicator: true,
	store.CoordinationTaskRoleResponder:    true,
}

// defaultTaskDueOffset returns the default due-at offset for a task kind.
var defaultTaskDueOffset = map[string]time.Duration{
	store.CoordinationTaskKindInvestigate: 30 * time.Minute,
	store.CoordinationTaskKindCommunicate: 10 * time.Minute,
	store.CoordinationTaskKindVerify:      15 * time.Minute,
	store.CoordinationTaskKindMitigate:    20 * time.Minute,
}

// performDispatchTask (commander-only) creates a CoordinationTask targeting a
// role (responder/communicator/commander) and anchors it with a root
// coordination message. The scheduler's scheduleCoordinationTasks picks up the
// pending task and dispatches it to an available agent holding that role.
func (e *AgentToolExecutor) performDispatchTask(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if e.coordinationTaskStore == nil {
		return errors.New("coordination task store not configured")
	}

	kind := strings.TrimSpace(strings.ToLower(cmd.TaskKind))
	if kind == "" {
		kind = store.CoordinationTaskKindInvestigate
	}
	if !validCoordinationTaskKinds[kind] {
		return fmt.Errorf("task_kind must be one of: %s, %s, %s, %s", store.CoordinationTaskKindInvestigate, store.CoordinationTaskKindCommunicate, store.CoordinationTaskKindVerify, store.CoordinationTaskKindMitigate)
	}

	role := strings.TrimSpace(strings.ToLower(cmd.AssigneeRole))
	if role == "" {
		role = store.CoordinationTaskRoleResponder
	}
	if !validCoordinationTaskRoles[role] {
		return fmt.Errorf("assignee_role must be one of: %s, %s, %s", store.CoordinationTaskRoleCommander, store.CoordinationTaskRoleCommunicator, store.CoordinationTaskRoleResponder)
	}

	goal := strings.TrimSpace(cmd.Goal)
	if goal == "" {
		return errors.New("goal is required")
	}

	record := &store.CoordinationTaskRecord{
		IncidentNumber:   incidentNumber,
		Kind:             kind,
		AssigneeRole:     role,
		AssigneeAgentID:  strings.TrimSpace(cmd.AssigneeAgentID),
		Goal:             goal,
		InputContext:     cmd.InputContext,
		CreatedByAgentID: agentRec.ID.String(),
		CreatedByName:    agentRec.Name,
		Status:           store.CoordinationTaskStatusPending,
	}
	if parent := strings.TrimSpace(cmd.ParentTaskID); parent != "" {
		parentID, perr := uuid.Parse(parent)
		if perr != nil {
			return fmt.Errorf("invalid parent_task_id: %w", perr)
		}
		record.ParentTaskID = &parentID
	}
	if record.DueAt == nil {
		if offset, ok := defaultTaskDueOffset[kind]; ok {
			due := time.Now().UTC().Add(offset)
			record.DueAt = &due
		}
	}

	created, err := e.coordinationTaskStore.CreateTask(ctx, record)
	if err != nil {
		return fmt.Errorf("create coordination task: %w", err)
	}

	taskIDStr := created.ID.String()

	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "coordination_task_created",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s dispatched %s task to %s role: %s", agentRec.Name, kind, role, goal),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"action":          "dispatch_task",
			"task_id":         taskIDStr,
			"kind":            kind,
			"assignee_role":   role,
		})
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "coordination_task_created", Data: map[string]any{
			"incident_number": incidentNumber,
			"task_id":         taskIDStr,
			"kind":            kind,
			"assignee_role":   role,
		}})
	}

	// Best-effort: anchor the task with a root coordination message so the
	// coordination thread reflects the dispatched work.
	if e.incidentCoordinationStore != nil {
		meta := map[string]any{
			"coordination_task_id":        taskIDStr,
			"linked_coordination_task_id": taskIDStr,
			"source_tool":                 "dispatch_task",
			"kind":                        kind,
			"assignee_role":               role,
		}
		if _, merr := e.incidentCoordinationStore.CreateMessage(ctx, &store.IncidentCoordinationMessageRecord{
			IncidentNumber:   incidentNumber,
			Kind:             store.IncidentCoordinationKindAction,
			ActorType:        store.IncidentCoordinationActorAgent,
			ActorID:          &agentRec.ID,
			ActorDisplayName: agentRec.Name,
			Body:             fmt.Sprintf("📋 Dispatched %s task to %s: %s", kind, role, goal),
			Source:           store.IncidentCoordinationSourceAgent,
			Metadata:         meta,
		}); merr != nil {
			logger.WarnCtx(ctx, "failed to anchor coordination task dispatch message", "incident_number", incID, "task_id", taskIDStr, "error", merr)
		}
	}
	return nil
}

// performClaimTask lets an agent proactively pull a pending task. The agent's
// claimable role is DERIVED from their active incident roles (not trusted from
// the request) so an agent cannot claim a task for a role they do not hold.
func (e *AgentToolExecutor) performClaimTask(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if e.coordinationTaskStore == nil {
		return errors.New("coordination task store not configured")
	}
	taskIDStr := strings.TrimSpace(cmd.TaskID)
	if taskIDStr == "" {
		return errors.New("task_id is required")
	}
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		return fmt.Errorf("invalid task_id: %w", err)
	}

	// Derive the agent's coordination-task role from their active incident roles.
	roles := e.activeAgentIncidentRoles(ctx, agentRec.ID, incidentNumber)
	var role string
	switch {
	case roles[string(ics.RoleIncidentCommander)]:
		role = store.CoordinationTaskRoleCommander
	case roles[string(ics.RoleCommunicationsLead)]:
		role = store.CoordinationTaskRoleCommunicator
	case roles[string(ics.RoleResponder)]:
		role = store.CoordinationTaskRoleResponder
	}
	if role == "" {
		// Fall back to the agent's strongest capability; required for agents that
		// are assigned as investigators without a formal ICS responder role.
		switch {
		case capability.Has(agent.Capabilities, capability.Command):
			role = store.CoordinationTaskRoleCommander
		case capability.Has(agent.Capabilities, capability.Communicate):
			role = store.CoordinationTaskRoleCommunicator
		case capability.Has(agent.Capabilities, capability.Investigate):
			role = store.CoordinationTaskRoleResponder
		default:
			return errors.New("agent has no incident role assigned")
		}
	}

	claimed, err := e.coordinationTaskStore.ClaimTask(ctx, taskID, role, agentRec.ID.String(), agentRec.Name)
	if err != nil {
		if errors.Is(err, store.ErrCoordinationTaskStatusConflict) {
			return errors.New("task already claimed or not pending for this role")
		}
		return fmt.Errorf("claim coordination task: %w", err)
	}

	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "coordination_task_claimed", Data: map[string]any{
			"incident_number": incidentNumber,
			"task_id":         claimed.ID.String(),
			"agent_id":        agentRec.ID.String(),
			"agent_name":      agentRec.Name,
			"assignee_role":   role,
		}})
	}
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"action":          "claim_task",
			"task_id":         claimed.ID.String(),
			"assignee_role":   role,
		})
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "coordination_task_claimed",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s claimed a %s task", agentRec.Name, claimed.Kind),
	})
	return nil
}

// performCompleteTask (assignee-only) verifies the caller is the task's
// assignee, then completes the task via the store's CAS + roll-up.
func (e *AgentToolExecutor) performCompleteTask(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if e.coordinationTaskStore == nil {
		return errors.New("coordination task store not configured")
	}
	taskIDStr := strings.TrimSpace(cmd.TaskID)
	if taskIDStr == "" {
		return errors.New("task_id is required")
	}
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		return fmt.Errorf("invalid task_id: %w", err)
	}

	task, err := e.coordinationTaskStore.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load coordination task: %w", err)
	}
	if task.AssigneeAgentID != agentRec.ID.String() {
		return errors.New("not the assignee of this task")
	}

	result := cmd.Result
	if result == nil {
		result = map[string]any{}
	}
	if err := e.coordinationTaskStore.CompleteTask(ctx, taskID, result); err != nil {
		if errors.Is(err, store.ErrCoordinationTaskStatusConflict) {
			return errors.New("task not in progress or not assigned to agent")
		}
		return fmt.Errorf("complete coordination task: %w", err)
	}

	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "coordination_task_completed", Data: map[string]any{
			"incident_number": incidentNumber,
			"task_id":         taskIDStr,
			"assignee_role":   task.AssigneeRole,
		}})
	}
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"action":          "complete_task",
			"task_id":         taskIDStr,
			"assignee_role":   task.AssigneeRole,
		})
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "coordination_task_completed",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s completed a %s task", agentRec.Name, task.Kind),
	})

	// Best-effort: post a threaded coordination reply summarizing the result.
	if e.incidentCoordinationStore != nil {
		summary := summarizeTaskResult(result)
		if strings.TrimSpace(summary) == "" {
			summary = fmt.Sprintf("✅ Completed %s task: %s", task.Kind, task.Goal)
		}
		if _, merr := e.incidentCoordinationStore.CreateMessage(ctx, &store.IncidentCoordinationMessageRecord{
			IncidentNumber:   incidentNumber,
			Kind:             store.IncidentCoordinationKindAgentReply,
			ActorType:        store.IncidentCoordinationActorAgent,
			ActorID:          &agentRec.ID,
			ActorDisplayName: agentRec.Name,
			Body:             summary,
			Source:           store.IncidentCoordinationSourceAgent,
			Metadata: map[string]any{
				"source_tool":          "complete_task",
				"coordination_task_id": taskIDStr,
				"kind":                 task.Kind,
			},
		}); merr != nil {
			logger.WarnCtx(ctx, "failed to post coordination task completion reply", "incident_number", incID, "task_id", taskIDStr, "error", merr)
		}
	}
	return nil
}

// summarizeTaskResult reduces a task result map to a short human-readable line
// for the coordination thread. Returns "" when no usable field is present.
func summarizeTaskResult(result map[string]any) string {
	if result == nil {
		return ""
	}
	if s, ok := result["summary"].(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	if s, ok := result["message"].(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	if s, ok := result["root_cause"].(string); ok && strings.TrimSpace(s) != "" {
		return fmt.Sprintf("Root cause: %s", s)
	}
	if s, ok := result["resolution"].(string); ok && strings.TrimSpace(s) != "" {
		return fmt.Sprintf("Resolution: %s", s)
	}
	return ""
}

// performSynthesizeFindings (commander-only) gathers the inputs (child
// investigation findings + completed coordination tasks) and writes the
// commander's synthesized summary to the coordinating investigation.
func (e *AgentToolExecutor) performSynthesizeFindings(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if !e.agentCanResolveIncident(ctx, agentRec, agent, incidentNumber) {
		return errors.New("only the active incident commander may synthesize findings")
	}
	if e.coordinationTaskStore == nil {
		return errors.New("coordination task store not configured")
	}
	if e.incidentInvestigationStore == nil {
		return errors.New("incident investigation store not configured")
	}

	// Find the coordinating (commander-owned) investigation.
	invs, err := e.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incidentNumber)
	if err != nil {
		return fmt.Errorf("list incident investigations: %w", err)
	}
	var coordinating *store.IncidentInvestigationRecord
	childCount := 0
	for i := range invs {
		if invs[i].Status == store.IncidentInvestigationStatusCoordinating {
			coordinating = &invs[i]
			continue
		}
		childCount++
	}
	if coordinating == nil {
		return errors.New("no coordinating investigation found")
	}

	// Gather completed coordination tasks for the incident.
	tasks, err := e.coordinationTaskStore.ListTasksByIncident(ctx, incidentNumber, map[string]any{
		"status": store.CoordinationTaskStatusComplete,
		"$limit": 200,
	})
	if err != nil {
		return fmt.Errorf("list completed coordination tasks: %w", err)
	}

	synthesized := &entschema.InvestigationSummary{
		Status:  "synthesized",
		Summary: strings.TrimSpace(cmd.Summary),
	}
	for i := range invs {
		if invs[i].Status == store.IncidentInvestigationStatusCoordinating {
			continue
		}
		for _, f := range invs[i].Findings {
			synthesized.Findings = append(synthesized.Findings, f.Title)
		}
		for _, ev := range invs[i].Evidence {
			synthesized.Evidence = append(synthesized.Evidence, ev.Content)
		}
	}
	for _, t := range tasks {
		if t.Result == nil {
			continue
		}
		if rc, ok := t.Result["root_cause"].(string); ok && strings.TrimSpace(rc) != "" && synthesized.RootCause == "" {
			synthesized.RootCause = rc
		}
		if res, ok := t.Result["resolution"].(string); ok && strings.TrimSpace(res) != "" && synthesized.Resolution == "" {
			synthesized.Resolution = res
		}
	}

	if err := e.incidentInvestigationStore.SetIncidentInvestigationSummary(ctx, coordinating.IncidentInvestigationID, synthesized); err != nil {
		return fmt.Errorf("set coordinating investigation summary: %w", err)
	}

	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_investigation_update", Data: map[string]any{
			"incident_number":      incidentNumber,
			"investigation_id":     coordinating.IncidentInvestigationID,
			"action":               "synthesize_findings",
			"child_investigations": childCount,
			"completed_tasks":      len(tasks),
		}})
	}
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number":      incID,
			"action":               "synthesize_findings",
			"investigation_id":     coordinating.IncidentInvestigationID,
			"child_investigations": childCount,
			"completed_tasks":      len(tasks),
		})
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "findings_synthesized",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s synthesized findings from %d child investigations and %d completed tasks", agentRec.Name, childCount, len(tasks)),
	})
	return nil
}
