package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"alga/capability"
	entschema "alga/ent/schema"
	"alga/ics"
	"alga/logger"
	"alga/store"
)

func (e *AgentToolExecutor) executeAlertOwnerTool(ctx context.Context, agentRec *store.AgentTokenRecord, chatID, ownerID string, cmd InvTool, op string) InvToolOutcome {
	if op != "resolve_alert" && op != "reopen_alert" {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "chat_id must be investigation_<id> for this tool"}
	}
	if e.alertSideEffects == nil || e.alertSideEffects.Store == nil {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "alert store not configured"}
	}
	alertNumber, err := strconv.ParseInt(ownerID, 10, 64)
	if err != nil || alertNumber <= 0 {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "invalid alert chat_id"}
	}
	alert, err := e.alertSideEffects.Store.GetByAlertNumber(alertNumber)
	if err != nil {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
	}
	if alert == nil {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "alert not found"}
	}
	if e.lifecycle != nil {
		if err := e.lifecycle.RequireAlertActionAllowed(ctx, alertNumber, agentRec); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
	}

	var isCommander bool
	if e.alertInvestigationStore != nil && e.incidentStore != nil {
		inv, _ := e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(ctx, alertNumber)
		if inv != nil && inv.PromotedIncidentID != nil {
			inc, err := e.incidentStore.GetIncidentByID(ctx, *inv.PromotedIncidentID)
			if err == nil && inc != nil {
				roles := e.activeAgentIncidentRoles(ctx, agentRec.ID, inc.IncidentNumber)
				// In incident scope, alert closure is part of incident closure and is owned by the
				// incident commander. Responders must not resolve alerts linked to an active
				// incident from the alert thread; the commander will close them as part of
				// alga_resolve_incident.
				if !roles[string(ics.RoleIncidentCommander)] {
					return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "only the incident commander is authorized to resolve or reopen alerts associated with an active incident; responders must let the commander close the alert as part of incident resolution"}
				}
				isCommander = true
			}
		}
	}

	agent := agentTokenContext{ID: agentRec.ID, Name: agentRec.Name, AgentType: agentRec.AgentType, Capabilities: agentRec.Capabilities}
	if isCommander {
		if err := e.requireCapability(agent, capability.Command); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
	} else {
		if err := e.requireCapability(agent, capability.Investigate); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
	}

	actor := agentActor(agentRec)
	alertLabel, _ := resolveAlertLabel(e.alertSideEffects.Store, alert.Fingerprint)
	var humanMsg string

	switch op {
	case "resolve_alert":
		if alert.Status != "resolved" {
			if err := e.alertSideEffects.Store.ResolveAlertByNumber(alertNumber, actor); err != nil {
				return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
			}
			if e.alertSideEffects.Dedup != nil {
				e.alertSideEffects.Dedup.RemoveTracking(ctx, alert.Fingerprint)
			}
		}
		var inv *store.AlertInvestigationRecord
		if e.alertInvestigationStore != nil {
			inv, _ = e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(ctx, alertNumber)
			if inv != nil {
				if cmd.RootCause != nil || cmd.Resolution != nil {
					_ = e.alertInvestigationStore.PatchAlertInvestigationOutcome(ctx, inv.ID.String(), cmd.RootCause, cmd.Resolution)
					if inv.Summary == nil {
						inv.Summary = &entschema.AlertInvestigationSummary{}
					}
					if cmd.RootCause != nil {
						inv.Summary.RootCause = *cmd.RootCause
					}
					if cmd.Resolution != nil {
						inv.Summary.Resolution = *cmd.Resolution
					}
				}
			}
		}
		// NOTE: do NOT call updateIncidentFromOutcome from resolve_alert even when the
		// alert-investigation has been promoted. The incident Summary card is owned by
		// the incident commander (alga_resolve_incident -> populateIncidentResolutionDocuments)
		// and writing it from the alert thread bypasses that ownership and breaks the
		// commander's executive summary.
		if e.lifecycle != nil {
			if err := e.lifecycle.CompleteIfAllAlertsResolved(ctx, store.AlertInvestigationLifecycleCompletionRequest{
				AlertNumber: alertNumber,
				Reason:      store.AlertInvestigationCompletedReasonAgentResolved,
				ActorType:   store.InvestigationActorAgent,
				ActorID:     agentRec.ID.String(),
				ActorName:   actor.Username,
			}); err != nil {
				logger.WarnCtx(ctx, "alert owner resolve: linked investigation completion failed", "alert_number", alertNumber, "error", err)
				return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "investigation status conflict"}
			}
			if inv != nil {
				inv.Status = store.AlertInvestigationStatusComplete
				e.extractMemories(inv)
			}
		}
		humanMsg = fmt.Sprintf("*%s* resolved alert %s", actor.Username, alertLabel)
	case "reopen_alert":
		ev := store.AlertEventWithActor("reopened", time.Now(), actor)
		if err := e.alertSideEffects.Store.ReopenAlertByNumber(alertNumber, ev); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
		if e.alertSideEffects.Dedup != nil {
			_ = e.alertSideEffects.Dedup.MarkTracked(ctx, alert.Fingerprint)
		}
		humanMsg = fmt.Sprintf("*%s* reopened alert %s", actor.Username, alertLabel)
	}

	e.afterAlertMutation(ctx, alert.Fingerprint, actor)
	e.ensureThreadMessage(ctx, store.ThreadOwnerAlert, ownerID, "action", humanMsg)
	e.logAudit(op, actor.Username, "", alert.Fingerprint)
	return InvToolOutcome{ChatID: chatID, Ok: true, Op: op}
}
