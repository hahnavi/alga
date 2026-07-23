package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"alga/ics"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

func authorizeAssignedAlertInvestigationAgent(agentRec *store.AgentTokenRecord, record *store.AlertInvestigationRecord) error {
	if record == nil {
		return store.ErrInvestigationNotFound
	}
	if agentRec == nil || record.AgentID != agentRec.ID.String() {
		return errors.New("not assigned to this investigation")
	}
	return nil
}

func (e *AgentToolExecutor) updateIncidentFromOutcome(ctx context.Context, incidentNumber int64, investigationID string, rootCause, resolution *string) {
	if e.incidentStore == nil || incidentNumber == 0 {
		return
	}
	inc, err := e.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		logger.WarnCtx(ctx, "failed to load incident for outcome update", "incident_number", incidentNumber, "investigation_id", investigationID, "error", err)
		return
	}

	parts := make([]string, 0, 2)
	if rootCause != nil && strings.TrimSpace(*rootCause) != "" {
		parts = append(parts, "Root cause: "+strings.TrimSpace(*rootCause))
	}
	if resolution != nil && strings.TrimSpace(*resolution) != "" {
		parts = append(parts, "Actions taken: "+strings.TrimSpace(*resolution))
		if inc.CustomFields == nil {
			inc.CustomFields = map[string]any{}
		}
		inc.CustomFields["actions_taken"] = strings.TrimSpace(*resolution)
	}
	if len(parts) == 0 {
		return
	}
	inc.Summary = strings.Join(parts, "\n")
	if _, err := e.incidentStore.UpdateIncident(ctx, incidentNumber, inc); err != nil {
		logger.WarnCtx(ctx, "failed to update incident from investigation outcome", "incident_number", incidentNumber, "investigation_id", investigationID, "error", err)
		return
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "incident_updated",
		ActorType:      "agent",
		Message:        "Incident summary updated from alert investigation outcome",
	})
}

func (e *AgentToolExecutor) finalizeAlertInvestigation(ctx context.Context, inv *store.AlertInvestigationRecord, investigationID, targetStatus, actorName string, actorID uuid.UUID) error {
	if inv == nil {
		return store.ErrInvestigationNotFound
	}
	if targetStatus == "" {
		targetStatus = store.AlertInvestigationStatusComplete
	}
	if err := e.alertInvestigationStore.TransitionAlertInvestigationStatus(ctx, inv.ID.String(), []string{"assigned", "investigating", "in_progress"}, targetStatus); err != nil {
		return err
	}
	inv.Status = targetStatus
	e.publishInvestigationStatusChange(investigationID, targetStatus)
	e.extractMemories(inv)
	if inv.PromotedIncidentID != nil && e.incidentStore != nil {
		e.recordIncidentInvestigationCompletion(ctx, inv, investigationID, actorName, actorID)
	}
	return nil
}

// backpropagateOutcomeToAlertInvestigations writes root_cause and resolution
// back to any alert investigations that were promoted to this incident but never
// had their outcome set (the common case: alert-scope agents promote immediately
// and stop, so alga_set_outcome is never called from the alert thread). It
// derives root_cause from the incident executive summary and resolution from the
// actions_taken document section. It is fire-and-forget: errors are logged and
// do not block resolution.
func (e *AgentToolExecutor) backpropagateOutcomeToAlertInvestigations(ctx context.Context, incidentNumber int64, inc *store.IncidentRecord, cmd InvTool) {
	if e.alertInvestigationStore == nil || inc == nil {
		return
	}

	// Root cause: use the incident's executive summary (already written to
	// inc.Summary by populateIncidentResolutionDocuments before this call).
	rootCauseStr := strings.TrimSpace(inc.Summary)

	// Resolution: prefer the inline cmd field; fall back to the stored
	// resolution or actions_taken document sections.
	resolutionStr := strings.TrimSpace(cmd.ActionsTaken)
	if resolutionStr == "" && e.incidentDocumentStore != nil {
		if sections, err := e.incidentDocumentStore.GetAllSections(ctx, incidentNumber); err == nil {
			for _, s := range sections {
				if s.Section == string(ics.SectionResolution) && strings.TrimSpace(s.Content) != "" {
					resolutionStr = strings.TrimSpace(s.Content)
					break
				}
			}
			if resolutionStr == "" {
				for _, s := range sections {
					if s.Section == string(ics.SectionActionsTaken) {
						resolutionStr = strings.TrimSpace(s.Content)
						break
					}
				}
			}
		}
	}

	if rootCauseStr == "" && resolutionStr == "" {
		return
	}

	var rootCause, resolution *string
	if rootCauseStr != "" {
		rootCause = &rootCauseStr
	}
	if resolutionStr != "" {
		resolution = &resolutionStr
	}

	invs, err := e.alertInvestigationStore.ListAlertInvestigations(ctx, map[string]any{"promoted_incident_id": inc.ID.String()})
	if err != nil {
		logger.WarnCtx(ctx, "backpropagate outcome: failed to list linked alert investigations", "incident_number", incidentNumber, "error", err)
		return
	}
	for i := range invs {
		inv := &invs[i]
		// Skip investigations that already have a root cause recorded.
		if inv.Summary != nil && strings.TrimSpace(inv.Summary.RootCause) != "" {
			continue
		}
		if err := e.alertInvestigationStore.PatchAlertInvestigationOutcome(ctx, inv.AlertInvestigationID, rootCause, resolution); err != nil {
			logger.WarnCtx(ctx, "backpropagate outcome: failed to patch alert investigation", "investigation_id", inv.AlertInvestigationID, "error", err)
		}
	}
}

func (e *AgentToolExecutor) finalizeIncidentRelatedInvestigations(ctx context.Context, incidentNumber int64, agentRec *store.AgentTokenRecord) {
	if e.alertInvestigationStore == nil || agentRec == nil || incidentNumber == 0 {
		return
	}
	inc, incErr := e.incidentStore.GetIncident(ctx, incidentNumber)
	if incErr != nil || inc == nil {
		return
	}
	invs, err := e.alertInvestigationStore.ListAlertInvestigations(ctx, map[string]any{"promoted_incident_id": inc.ID.String()})
	if err != nil {
		logger.WarnCtx(ctx, "failed to list incident-related investigations for finalization", "incident_number", incidentNumber, "error", err)
		return
	}
	actorName := strings.TrimSpace(agentRec.Name)
	if actorName == "" {
		actorName = "agent"
	}
	for i := range invs {
		inv := &invs[i]
		if store.IsTerminalInvestigationStatus(inv.Status) || inv.Status == store.AlertInvestigationStatusComplete {
			continue
		}
		if inv.AgentID != "" && inv.AgentID != agentRec.ID.String() {
			continue
		}
		if err := e.finalizeAlertInvestigation(ctx, inv, inv.AlertInvestigationID, store.AlertInvestigationStatusComplete, actorName, agentRec.ID); err != nil {
			logger.WarnCtx(ctx, "failed to finalize incident-related investigation", "incident_number", incidentNumber, "investigation_id", inv.AlertInvestigationID, "error", err)
		}
	}
}

func (e *AgentToolExecutor) recordIncidentInvestigationCompletion(ctx context.Context, inv *store.AlertInvestigationRecord, investigationID, actorName string, actorID uuid.UUID) {
	var incidentNumber int64
	var incidentIDStr string
	if inv.PromotedIncidentID != nil {
		incidentIDStr = inv.PromotedIncidentID.String()
		if e.incidentStore != nil {
			if inc, err := e.incidentStore.GetIncidentByID(ctx, *inv.PromotedIncidentID); err == nil && inc != nil {
				incidentNumber = inc.IncidentNumber
			}
		}
	}
	if e.incidentStore == nil || inv == nil || incidentIDStr == "" {
		return
	}
	latest, err := e.alertInvestigationStore.GetAlertInvestigation(ctx, investigationID)
	if err == nil && latest != nil {
		inv = latest
	}
	var rootCause, resolution *string
	if inv.Summary != nil {
		if strings.TrimSpace(inv.Summary.RootCause) != "" {
			rc := inv.Summary.RootCause
			rootCause = &rc
		}
		if strings.TrimSpace(inv.Summary.Resolution) != "" {
			res := inv.Summary.Resolution
			resolution = &res
		}
	}
	if incidentNumber != 0 {
		e.updateIncidentFromOutcome(ctx, incidentNumber, investigationID, rootCause, resolution)

		timelineMsg := fmt.Sprintf("Alert investigation completed by agent %s", actorName)
		if inv.Summary != nil && inv.Summary.RootCause != "" {
			timelineMsg += fmt.Sprintf(" — Root cause: %s", inv.Summary.RootCause)
		}
		_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
			IncidentNumber: incidentNumber,
			EventType:      "investigation_completed",
			ActorID:        &actorID,
			ActorType:      "agent",
			Message:        timelineMsg,
		})
		if e.incidentCoordinationStore != nil && inv.Summary != nil {
			summaryText := strings.TrimSpace(inv.Summary.Summary)
			if summaryText == "" {
				summaryText = strings.TrimSpace(strings.Join([]string{inv.Summary.RootCause, inv.Summary.Resolution}, "\n"))
			}
			if summaryText != "" {
				_, _ = e.incidentCoordinationStore.CreateMessage(ctx, &store.IncidentCoordinationMessageRecord{
					IncidentNumber:        incidentNumber,
					Kind:                  store.IncidentCoordinationKindInvestigationSummary,
					ActorType:             store.IncidentCoordinationActorSystem,
					ActorDisplayName:      "System",
					Body:                  summaryText,
					Source:                store.IncidentCoordinationSourceSystem,
					LinkedInvestigationID: investigationID,
					Metadata:              map[string]any{"summary_source": "investigation"},
				})
			}
		}
		if e.ssePublisher != nil {
			e.ssePublisher.Publish(sse.Event{Type: "incident_updated", Data: map[string]string{"incident_number": incidentIDStr}})
		}
	}
}

func (e *AgentToolExecutor) EnsurePostMortem(ctx context.Context, incidentNumber int64, summary string) {
	if e.postmortemStore == nil || e.incidentStore == nil || incidentNumber == 0 {
		return
	}
	inc, err := e.incidentStore.GetIncident(ctx, incidentNumber)
	if err != nil || inc == nil {
		logger.WarnCtx(ctx, "failed to load incident for post-mortem", "incident_number", incidentNumber, "error", err)
		return
	}
	existing, err := e.postmortemStore.GetByIncidentID(ctx, inc.ID)
	if err != nil {
		logger.WarnCtx(ctx, "failed to check post-mortem", "incident_number", incidentNumber, "error", err)
		return
	}
	if existing != nil {
		return
	}

	var alertStore store.Store
	if e.alertSideEffects != nil {
		alertStore = e.alertSideEffects.Store
	}

	if e.buildPostMortemDraftFn == nil {
		return
	}
	draft := e.buildPostMortemDraftFn(ctx, e.incidentDocumentStore, e.incidentCoordinationStore, e.incidentStore, alertStore, inc, summary)

	_, err = e.postmortemStore.Create(ctx, draft)
	if err != nil {
		logger.WarnCtx(ctx, "failed to auto-create post-mortem", "incident_number", incidentNumber, "error", err)
		return
	}
	_ = e.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      "postmortem_created",
		ActorType:      "system",
		Message:        "Post-mortem created",
	})
}
