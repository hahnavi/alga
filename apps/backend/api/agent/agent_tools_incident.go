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
	"alga/ics"
	"alga/incident"
	"alga/logger"
	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
	"alga/strutil"
	"alga/worker"
)

func (e *AgentToolExecutor) executeIncidentTool(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, chatID string, cmd InvTool, op string) InvToolOutcome {
	if e.incidentStore == nil {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "incident store not configured"}
	}
	incidentNumber := cmd.IncidentNumber
	if incidentNumber == 0 {
		parsed, ok := incidentNumberFromIncidentChatID(chatID)
		if !ok {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "incident_number is required"}
		}
		incidentNumber = parsed
	}
	if incidentNumber == 0 {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "incident_number is required"}
	}
	if err := e.authorizeIncidentTool(ctx, agentRec, incidentNumber, op); err != nil {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
	}

	var err error
	switch op {
	case "set_incident_priority":
		err = e.performSetIncidentPriority(ctx, agentRec, agent, incidentNumber, cmd.Priority)
	case "set_incident_severity":
		err = e.performSetIncidentSeverity(ctx, agentRec, agent, incidentNumber, cmd.Severity)
	case "trigger_escalation":
		err = e.performTriggerEscalation(ctx, agentRec, agent, incidentNumber)
	case "mitigate_incident":
		err = e.performMitigateIncident(ctx, agentRec, agent, incidentNumber, cmd.Reason)
	case "resolve_incident":
		err = e.performResolveIncident(ctx, agentRec, agent, incidentNumber, cmd)
	case "begin_triage":
		err = e.performBeginTriage(ctx, agentRec, agent, incidentNumber)
	case "promote_incident":
		err = e.performPromoteIncident(ctx, agentRec, agent, incidentNumber)
	case "assign_incident_role":
		err = e.performAssignIncidentRole(ctx, agentRec, agent, incidentNumber, cmd)
	case "set_incident_resolution_docs":
		err = e.performSetIncidentResolutionDocs(ctx, agentRec, agent, incidentNumber, cmd)
	case "pause_investigation":
		err = e.performIncidentInvestigationPause(ctx, agentRec, agent, incidentNumber)
	case "cancel_investigation":
		err = e.performIncidentInvestigationCancel(ctx, agentRec, agent, incidentNumber)
	case "post_handoff":
		err = e.performPostHandoff(ctx, agentRec, agent, incidentNumber, cmd)
	case "publish_status_update":
		err = e.performPublishStatusUpdate(ctx, agentRec, agent, incidentNumber, cmd)
	case "dispatch_task":
		err = e.performDispatchTask(ctx, agentRec, agent, incidentNumber, cmd)
	case "claim_task":
		err = e.performClaimTask(ctx, agentRec, agent, incidentNumber, cmd)
	case "complete_task":
		err = e.performCompleteTask(ctx, agentRec, agent, incidentNumber, cmd)
	case "synthesize_findings":
		err = e.performSynthesizeFindings(ctx, agentRec, agent, incidentNumber, cmd)
	default:
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "unknown op"}
	}

	if err != nil {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
	}
	return InvToolOutcome{ChatID: chatID, Ok: true, Op: op}
}

func (e *AgentToolExecutor) performSetIncidentPriority(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, priority string) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if e.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	validPriorities := map[string]bool{"P1": true, "P2": true, "P3": true, "P4": true, "P5": true}
	if !validPriorities[priority] {
		return errors.New("priority must be P1-P5")
	}
	inc, err := e.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		return fmt.Errorf("incident %s not found", incID)
	}
	respondDur, resolveDur := worker.PriorityToSLATargets(priority)
	now := time.Now().UTC()
	respondAt := now.Add(respondDur)
	resolveAt := now.Add(resolveDur)
	inc.Priority = priority
	inc.SLATargetRespondAt = &respondAt
	inc.SLATargetResolveAt = &resolveAt
	if _, err := e.incidentStore.UpdateIncident(ctx, incidentNumber, inc); err != nil {
		return fmt.Errorf("update incident: %w", err)
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "incident_updated",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s set priority to %s", agentRec.Name, priority),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"field":           "priority",
			"value":           priority,
		})
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "priority": priority}})
	}
	return nil
}

func (e *AgentToolExecutor) performSetIncidentSeverity(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, severity string) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireAnyCapability(agent, capability.Investigate, capability.Command); err != nil {
		return err
	}
	if e.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	severity = strings.ToLower(strings.TrimSpace(severity))
	validSeverities := map[string]bool{"critical": true, "high": true, "warning": true, "info": true}
	if !validSeverities[severity] {
		return errors.New("severity must be one of: critical, high, warning, info")
	}
	inc, err := e.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		return fmt.Errorf("incident %s not found", incID)
	}
	inc.Severity = severity
	if _, err := e.incidentStore.UpdateIncident(ctx, incidentNumber, inc); err != nil {
		return fmt.Errorf("update incident: %w", err)
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "incident_updated",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s set severity to %s", agentRec.Name, severity),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"field":           "severity",
			"value":           severity,
		})
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "severity": severity}})
	}
	return nil
}

func (e *AgentToolExecutor) performTriggerEscalation(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if e.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	inc, err := e.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		return fmt.Errorf("incident %s not found", incID)
	}
	var policyID uuid.UUID
	if inc.EscalationPolicyID != nil {
		policyID = *inc.EscalationPolicyID
	} else if inc.ServiceID != nil && e.serviceStore != nil {
		svc, svcErr := e.serviceStore.GetService(ctx, inc.ServiceID.String())
		if svcErr == nil && svc != nil && svc.EscalationPolicyID != nil {
			policyID = *svc.EscalationPolicyID
		}
	}
	if policyID == uuid.Nil {
		return errors.New("no escalation policy found for incident")
	}
	if e.escalationPublisher != nil {
		escMsg := rabbitmq.EscalationMessage{
			IncidentNumber: incidentNumber,
			PolicyID:       policyID,
			Level:          1,
			MaxRetries:     rabbitmq.MaxEscalationRetries,
		}
		if pubErr := e.escalationPublisher.PublishEscalation(ctx, escMsg); pubErr != nil {
			return fmt.Errorf("publish escalation: %w", pubErr)
		}
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "incident_escalated",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s triggered escalation", agentRec.Name),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentEscalated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"policy_id":       policyID.String(),
		})
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "action": "escalated"}})
	}
	return nil
}

func (e *AgentToolExecutor) performMitigateIncident(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, reason string) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if e.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	if err := e.incidentStore.TransitionIncidentStatus(ctx, incidentNumber, []string{"detected", "triaging", "active"}, "mitigated"); err != nil {
		return fmt.Errorf("transition to mitigated: %w", err)
	}
	if reason == "" {
		reason = "Mitigated by agent"
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "incident_mitigated",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s mitigated incident: %s", agentRec.Name, reason),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentMitigated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
		})
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "status": "mitigated"}})
	}
	e.finalizeIncidentRelatedInvestigations(ctx, incidentNumber, agentRec)
	return nil
}

func (e *AgentToolExecutor) performResolveIncident(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if !e.agentCanResolveIncident(ctx, agentRec, agent, incidentNumber) {
		return errors.New("investigator agents cannot resolve incidents directly; notify the incident commander for verification")
	}
	inc, err := e.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		return fmt.Errorf("incident %s not found", incID)
	}
	if err := e.populateIncidentResolutionDocuments(ctx, agentRec, inc, cmd); err != nil {
		return err
	}
	missing, err := e.missingIncidentResolutionRequirements(ctx, inc)
	if err != nil {
		return fmt.Errorf("validate resolution requirements: %w", err)
	}
	if len(missing) > 0 {
		return fmt.Errorf("incident resolution requires: %s", strings.Join(missing, ", "))
	}
	if err := e.incidentStore.TransitionIncidentStatus(ctx, incidentNumber, []string{"detected", "triaging", "active", "mitigated"}, "resolved"); err != nil {
		return fmt.Errorf("transition to resolved: %w", err)
	}
	reason := cmd.Reason
	if reason == "" {
		reason = "Resolved by agent"
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "incident_resolved",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s resolved incident: %s", agentRec.Name, reason),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentResolved, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
		})
	}
	e.EnsurePostMortem(ctx, incidentNumber, "Incident resolved by agent "+agentRec.Name)
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "status": "resolved"}})
	}
	e.backpropagateOutcomeToAlertInvestigations(ctx, incidentNumber, inc, cmd)
	e.finalizeIncidentRelatedInvestigations(ctx, incidentNumber, agentRec)

	if e.alertSideEffects != nil && e.alertSideEffects.Store != nil && e.runAlertCascadeFn != nil {
		e.runAlertCascadeFn(ctx, e.alertSideEffects.Store, e.auditStore, e.ssePublisher, incidentNumber, agentRec.ID, agentRec.Name)
	}
	return nil
}

// performSetIncidentResolutionDocs records the commander-authored resolution
// documents (summary, impact assessment, actions taken, root cause, and
// resolution) without transitioning the incident. Commander-only; the same
// fields also can be supplied inline to resolve_incident. root_cause and
// resolution are mandatory incident document sections before an incident can
// be resolved.
func (e *AgentToolExecutor) performSetIncidentResolutionDocs(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	var rootCauseVal, resolutionVal string
	if cmd.RootCause != nil {
		rootCauseVal = strings.TrimSpace(*cmd.RootCause)
	}
	if cmd.Resolution != nil {
		resolutionVal = strings.TrimSpace(*cmd.Resolution)
	}
	if strings.TrimSpace(cmd.Summary) == "" &&
		strings.TrimSpace(cmd.ImpactAssessment) == "" &&
		strings.TrimSpace(cmd.ActionsTaken) == "" &&
		rootCauseVal == "" && resolutionVal == "" {
		return errors.New("at least one of summary, impact_assessment, actions_taken, root_cause, or resolution is required")
	}
	inc, err := e.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		return fmt.Errorf("incident %s not found", incID)
	}
	if err := e.populateIncidentResolutionDocuments(ctx, agentRec, inc, cmd); err != nil {
		return err
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "resolution_docs_updated",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s updated resolution documents", agentRec.Name),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"action":          "set_incident_resolution_docs",
		})
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "action": "resolution_docs_updated"}})
	}
	return nil
}

// populateIncidentResolutionDocuments writes any non-empty resolution fields
// from cmd to the incident summary and the impact_assessment, actions_taken,
// root_cause, and resolution document sections. The summary is overwritten
// when provided (matching the existing agent summary precedent) so both
// set_incident_resolution_docs and inline resolve_incident fields
// authoritatively record the resolution summary. Errors writing individual
// sections are logged and do not abort: the caller (resolve) re-validates the
// missing-field requirements and reports precisely what is still absent.
func (e *AgentToolExecutor) populateIncidentResolutionDocuments(ctx context.Context, agentRec *store.AgentTokenRecord, inc *store.IncidentRecord, cmd InvTool) error {
	if inc == nil {
		return nil
	}
	summary := strings.TrimSpace(cmd.Summary)
	if summary != "" {
		inc.Summary = summary
		if _, err := e.incidentStore.UpdateIncident(ctx, inc.IncidentNumber, inc); err != nil {
			return fmt.Errorf("update incident summary: %w", err)
		}
	}
	if e.incidentDocumentStore == nil {
		return nil
	}
	var rootCauseVal string
	if cmd.RootCause != nil {
		rootCauseVal = *cmd.RootCause
	}
	var resolutionVal string
	if cmd.Resolution != nil {
		resolutionVal = *cmd.Resolution
	}
	for _, s := range []struct {
		section ics.DocumentSection
		content string
	}{
		{ics.SectionImpactAssessment, strings.TrimSpace(cmd.ImpactAssessment)},
		{ics.SectionActionsTaken, strings.TrimSpace(cmd.ActionsTaken)},
		{ics.SectionRootCause, strings.TrimSpace(rootCauseVal)},
		{ics.SectionResolution, strings.TrimSpace(resolutionVal)},
	} {
		if s.content == "" {
			continue
		}
		if err := e.upsertAgentIncidentDocSection(ctx, inc.IncidentNumber, s.section, s.content); err != nil {
			logger.WarnCtx(ctx, "failed to upsert incident document section from agent", "incident_number", inc.IncidentNumber, "section", string(s.section), "error", err)
		}
	}
	return nil
}

// upsertAgentIncidentDocSection writes a document section as an agent-authored
// edit (no user editor). It fetches the current version first so an existing
// section is updated without a version conflict.
func (e *AgentToolExecutor) upsertAgentIncidentDocSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection, content string) error {
	if e.incidentDocumentStore == nil {
		return errors.New("incident document store not configured")
	}
	version := 0
	if existing, err := e.incidentDocumentStore.GetSection(ctx, incidentNumber, section); err != nil {
		return fmt.Errorf("load section %s: %w", string(section), err)
	} else if existing != nil {
		version = existing.Version
	}
	if _, err := e.incidentDocumentStore.UpsertSection(ctx, incidentNumber, section, content, version, uuid.Nil); err != nil {
		return fmt.Errorf("upsert section %s: %w", string(section), err)
	}
	return nil
}

func (e *AgentToolExecutor) agentCanResolveIncident(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64) bool {
	if agentRec == nil || !capability.Has(agent.Capabilities, capability.Command) || e.icsRoleStore == nil {
		return false
	}
	roles, err := e.icsRoleStore.GetActiveRoles(ctx, incidentNumber)
	if err != nil {
		logger.WarnCtx(ctx, "failed to check commander role for agent resolution", "incident_number", incidentNumber, "agent_id", agentRec.ID.String(), "error", err)
		return false
	}
	for _, role := range roles {
		if role.RoleType == string(ics.RoleIncidentCommander) && role.AssigneeType == "agent" && role.AgentTokenID != nil && *role.AgentTokenID == agentRec.ID && role.Status == string(ics.RoleStatusActive) {
			return true
		}
	}
	return false
}

func (e *AgentToolExecutor) missingIncidentResolutionRequirements(ctx context.Context, inc *store.IncidentRecord) ([]string, error) {
	missing := []string{}
	if inc == nil || strings.TrimSpace(inc.Summary) == "" {
		missing = append(missing, "summary")
	}
	// When an agent communicator is assigned, the commander must not silently
	// bypass them: resolution requires at least one published public status
	// update so the communicator is visibly involved in the incident.
	if inc != nil && e.incidentCoordinationStore != nil {
		updates, listErr := e.incidentCoordinationStore.ListMessagesByKind(ctx, inc.IncidentNumber, store.IncidentCoordinationKindStatusUpdate, 50, 0)
		if listErr != nil {
			return nil, fmt.Errorf("check published status updates: %w", listErr)
		}
		if !hasResolvedStatusUpdate(updates) {
			missing = append(missing, "a resolved status update (publish one via alga_publish_status_update with status_level=resolved)")
		}
	}
	if e.incidentDocumentStore == nil {
		missing = append(missing, "impact_assessment", "root_cause", "resolution")
		return missing, nil
	}
	sections, err := e.incidentDocumentStore.GetAllSections(ctx, inc.IncidentNumber)
	if err != nil {
		return nil, err
	}
	contentBySection := map[string]string{}
	for _, section := range sections {
		contentBySection[section.Section] = strings.TrimSpace(section.Content)
	}
	if contentBySection["impact_assessment"] == "" {
		missing = append(missing, "impact_assessment")
	}
	if contentBySection["root_cause"] == "" {
		missing = append(missing, "root_cause")
	}
	if contentBySection["resolution"] == "" {
		missing = append(missing, "resolution")
	}
	return missing, nil
}

func hasResolvedStatusUpdate(updates []store.IncidentCoordinationMessageRecord) bool {
	for _, update := range updates {
		if coordinationStatusLevel(update) == "resolved" {
			return true
		}
	}
	return false
}

func coordinationStatusLevel(update store.IncidentCoordinationMessageRecord) string {
	if update.Metadata == nil {
		return ""
	}
	level, _ := update.Metadata["status_level"].(string)
	return normalizeCoordinationStatusLevel(level)
}

func (e *AgentToolExecutor) performBeginTriage(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if e.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	if err := e.incidentStore.TransitionIncidentStatus(ctx, incidentNumber, []string{"detected"}, "triaging"); err != nil {
		return fmt.Errorf("transition to triaging: %w", err)
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "triage_started",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s started triage", agentRec.Name),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"status":          "triaging",
		})
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "status": "triaging"}})
	}
	return nil
}

func (e *AgentToolExecutor) performPromoteIncident(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if e.incidentStore == nil {
		return errors.New("incident store not configured")
	}
	if err := e.incidentStore.TransitionIncidentStatus(ctx, incidentNumber, []string{"triaging"}, "active"); err != nil {
		return fmt.Errorf("promote incident: %w", err)
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "incident_promoted",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        fmt.Sprintf("Agent %s promoted incident to active", agentRec.Name),
	})
	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentUpdated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_number": incID,
			"status":          "active",
		})
	}
	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID, "status": "active"}})
	}
	return nil
}

func (e *AgentToolExecutor) performPromoteToIncident(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, chatID string, inv *store.AlertInvestigationRecord, cmd InvTool) (promotedIncidentOutcome, error) {
	if err := e.requireCapability(agent, capability.Investigate); err != nil {
		return promotedIncidentOutcome{}, err
	}
	if e.incidentStore == nil {
		return promotedIncidentOutcome{}, errors.New("incident store not configured")
	}
	if e.alertInvestigationStore == nil {
		return promotedIncidentOutcome{}, errors.New("alert investigation store not configured")
	}
	if e.incidentInvestigationStore == nil {
		return promotedIncidentOutcome{}, errors.New("incident investigation store not configured")
	}

	if inv.PromotedIncidentID != nil {
		return promotedIncidentOutcome{}, fmt.Errorf("already promoted to incident %s", inv.PromotedIncidentID.String())
	}

	firingFingerprints, resolvedFingerprints, checkErr := e.linkedAlertsLiveState(ctx, inv)
	if checkErr != nil {
		return promotedIncidentOutcome{}, checkErr
	}
	if len(firingFingerprints) == 0 {
		return promotedIncidentOutcome{}, fmt.Errorf(
			"refusing to promote: all %d linked alert(s) are no longer firing (resolved fingerprints: %s); "+
				"finalize this investigation with alga_set_outcome instead of opening an incident for a self-healed or already-resolved alert",
			len(inv.Alerts), strings.Join(resolvedFingerprints, ","),
		)
	}

	// Determine title
	title := cmd.Title
	if title == "" {
		if len(inv.Alerts) > 0 {
			if alertName, ok := inv.Alerts[0].Labels["alertname"]; ok && alertName != "" {
				title = fmt.Sprintf("Incident: %s", alertName)
			} else {
				title = fmt.Sprintf("Incident from Alert %s", strutil.Prefix(inv.Alerts[0].Fingerprint, 8))
			}
		} else {
			title = fmt.Sprintf("Incident from Alert Investigation %s", inv.AlertInvestigationID)
		}
	}

	// Determine Description (borrowing the summary of the alert investigation)
	var description string
	if inv.Summary != nil && inv.Summary.Summary != "" {
		description = inv.Summary.Summary
	}
	if description == "" {
		if len(inv.Alerts) > 0 {
			if desc, ok := inv.Alerts[0].Annotations["description"]; ok && desc != "" {
				description = desc
			} else if summ, ok := inv.Alerts[0].Annotations["summary"]; ok && summ != "" {
				description = summ
			}
		}
	}
	if description == "" {
		description = "No investigation summary available."
	}

	// Reserve incident ID
	incidentNumber, err := e.incidentStore.ReserveIncidentNumber(ctx)
	if err != nil {
		return promotedIncidentOutcome{}, fmt.Errorf("reserve incident number: %w", err)
	}

	severity := cmd.Severity
	if severity == "" {
		severity = "warning"
	}
	priority := cmd.Priority
	if priority == "" {
		priority = incident.ComputePriority(severity, "medium")
	}

	now := time.Now().UTC()
	record := &store.IncidentRecord{
		IncidentNumber: incidentNumber,
		Title:          title,
		Description:    description,
		Status:         "active",
		Severity:       severity,
		ImpactLevel:    "medium",
		Priority:       priority,
		StartedAt:      &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	created, err := e.incidentStore.CreateIncident(ctx, record)
	if err != nil {
		return promotedIncidentOutcome{}, fmt.Errorf("create incident: %w", err)
	}

	// Create incident investigation
	incidentInv, err := e.incidentInvestigationStore.CreateIncidentInvestigation(ctx, store.IncidentInvestigationRecord{
		IncidentNumber: created.IncidentNumber,
		Status:         "pending",
	})
	if err != nil {
		return promotedIncidentOutcome{}, fmt.Errorf("create incident investigation: %w", err)
	}

	// Mark alert investigation promoted
	_, err = e.alertInvestigationStore.MarkAlertInvestigationPromoted(ctx, inv.AlertInvestigationID, created.ID.String(), created.IncidentNumber, incidentInv.ID.String())
	if err != nil {
		return promotedIncidentOutcome{}, fmt.Errorf("mark alert investigation promoted: %w", err)
	}

	// Link all alerts
	for _, alert := range inv.Alerts {
		if e.alertSideEffects != nil && e.alertSideEffects.Store != nil {
			_ = e.alertSideEffects.Store.LinkAlertToIncident(ctx, alert.Fingerprint, created.IncidentNumber)
		}
	}

	// Add timeline entry to the new incident. Reference the alert by number —
	// the alert investigation id is an internal UUID that is not user-facing
	// or linkable, and the agent echoes timeline entries into its messages.
	creationMsg := "Incident created from an alert investigation"
	if inv.PrimaryAlertNumber > 0 {
		creationMsg = fmt.Sprintf("Incident created from alert #%d", inv.PrimaryAlertNumber)
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: created.IncidentNumber,
		EventType:      "created",
		ActorID:        &agentRec.ID,
		ActorType:      "agent",
		Message:        creationMsg,
	})

	if e.auditStore != nil {
		e.auditStore.Log(store.AuditIncidentCreated, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
			"incident_id":            created.IncidentNumber,
			"alert_investigation_id": inv.AlertInvestigationID,
			"linked_alerts_total":    len(inv.Alerts),
			"linked_alerts_firing":   len(firingFingerprints),
			"linked_alerts_resolved": len(resolvedFingerprints),
			"firing_fingerprints":    firingFingerprints,
		})
	}

	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_created", Data: created})
	}

	return promotedIncidentOutcome{
		IncidentNumber:          created.IncidentNumber,
		IncidentInvestigationID: incidentInv.ID.String(),
	}, nil
}

func (e *AgentToolExecutor) linkedAlertsLiveState(_ context.Context, inv *store.AlertInvestigationRecord) (firing, resolved []string, err error) {
	if e.alertSideEffects == nil || e.alertSideEffects.Store == nil {
		return nil, nil, errors.New("alert store not configured")
	}
	if len(inv.Alerts) == 0 {
		return nil, nil, nil
	}
	alerts := e.alertSideEffects.Store
	for _, alert := range inv.Alerts {
		fp := strings.TrimSpace(alert.Fingerprint)
		if fp == "" {
			continue
		}
		rec, getErr := alerts.GetByFingerprint(fp)
		if getErr != nil {
			return nil, nil, fmt.Errorf("live-state check failed for fingerprint %q: %w", fp, getErr)
		}
		if rec == nil {
			resolved = append(resolved, fp)
			continue
		}
		if rec.Status == "resolved" {
			resolved = append(resolved, fp)
			continue
		}
		firing = append(firing, fp)
	}
	return firing, resolved, nil
}

func (e *AgentToolExecutor) performAssignIncidentRole(ctx context.Context, agentRec *store.AgentTokenRecord, agent agentTokenContext, incidentNumber int64, cmd InvTool) error {
	incID := strconv.FormatInt(incidentNumber, 10)
	if err := e.requireCapability(agent, capability.Command); err != nil {
		return err
	}
	if e.icsRoleStore == nil {
		return errors.New("ICS role store not configured")
	}
	if cmd.RoleType == "" {
		return errors.New("role_type is required")
	}
	if !ics.ValidRoleType(ics.RoleType(cmd.RoleType)) {
		return fmt.Errorf("invalid role_type %q", cmd.RoleType)
	}
	if cmd.UserID == "" && cmd.AgentTokenID == "" {
		return errors.New("user_id or agent_token_id is required")
	}
	if cmd.UserID != "" && cmd.AgentTokenID != "" {
		return errors.New("provide either user_id or agent_token_id, not both")
	}

	if cmd.AgentTokenID != "" {
		atid, err := uuid.Parse(cmd.AgentTokenID)
		if err != nil {
			return errors.New("invalid agent_token_id")
		}
		_, err = e.icsRoleStore.AssignAgentRole(ctx, incidentNumber, ics.RoleType(cmd.RoleType), atid, nil, cmd.ScopeDescription)
		if err != nil {
			return fmt.Errorf("assign agent role: %w", err)
		}
		if e.auditStore != nil {
			e.auditStore.Log(store.AuditAgentRoleAssigned, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
				"incident_number": incID,
				"role_type":       cmd.RoleType,
				"agent_token_id":  atid.String(),
			})
		}
		_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
			IncidentNumber: incidentNumber,
			EventType:      "ics_role_assigned",
			ActorID:        &agentRec.ID,
			ActorType:      "agent",
			Message:        fmt.Sprintf("Agent ICS role assigned: %s", cmd.RoleType),
		})
	} else {
		uid, err := uuid.Parse(cmd.UserID)
		if err != nil {
			return errors.New("invalid user_id")
		}
		_, err = e.icsRoleStore.AssignRole(ctx, incidentNumber, ics.RoleType(cmd.RoleType), uid, nil, cmd.ScopeDescription)
		if err != nil {
			return fmt.Errorf("assign role: %w", err)
		}
		if e.auditStore != nil {
			e.auditStore.Log(store.AuditIncidentRoleAssigned, &agentRec.ID, agentRec.Name, "", "", true, map[string]any{
				"incident_number": incID,
				"role_type":       cmd.RoleType,
				"user_id":         uid.String(),
			})
		}
		_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
			IncidentNumber: incidentNumber,
			EventType:      "ics_role_assigned",
			ActorID:        &agentRec.ID,
			ActorType:      "agent",
			Message:        fmt.Sprintf("ICS role assigned: %s", cmd.RoleType),
		})
	}

	if e.ssePublisher != nil {
		e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incID}})
	}
	return nil
}
