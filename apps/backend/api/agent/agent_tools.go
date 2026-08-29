package agent

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/capability"
	"alga/ics"
	"alga/logger"
	"alga/mattermost"
	"alga/oncall"
	"alga/rabbitmq"
	"alga/slack"
	"alga/sse"
	"alga/store"
	"alga/valkey"
	"alga/webhook"
)

type InvTool struct {
	Op                          string         `json:"op"`
	ChatID                      string         `json:"chat_id"`
	Fingerprint                 string         `json:"fingerprint,omitempty"`
	RootCause                   *string        `json:"root_cause,omitempty"`
	Resolution                  *string        `json:"resolution,omitempty"`
	Reason                      string         `json:"reason,omitempty"`
	TriageResultID              string         `json:"triage_result_id,omitempty"`
	Agreed                      bool           `json:"agreed,omitempty"`
	CorrectDecision             string         `json:"correct_decision,omitempty"`
	CorrectSeverity             string         `json:"correct_severity,omitempty"`
	Note                        string         `json:"note,omitempty"`
	Message                     string         `json:"message,omitempty"`
	Audience                    string         `json:"audience,omitempty"`
	Urgency                     string         `json:"urgency,omitempty"`
	StatusLevel                 string         `json:"status_level,omitempty"`
	TargetAgentID               string         `json:"target_agent_id,omitempty"`
	IncidentNumber              int64          `json:"incident_number,omitempty"`
	Priority                    string         `json:"priority,omitempty"`
	Severity                    string         `json:"severity,omitempty"`
	RoleType                    string         `json:"role_type,omitempty"`
	UserID                      string         `json:"user_id,omitempty"`
	AgentTokenID                string         `json:"agent_token_id,omitempty"`
	ScopeDescription            *string        `json:"scope_description,omitempty"`
	Title                       string         `json:"title,omitempty"`
	Summary                     string         `json:"summary,omitempty"`
	ImpactAssessment            string         `json:"impact_assessment,omitempty"`
	ActionsTaken                string         `json:"actions_taken,omitempty"`
	ETADetail                   string         `json:"eta,omitempty"`
	SourceCoordinationMessageID string         `json:"source_coordination_message_id,omitempty"`
	Internal                    bool           `json:"internal,omitempty"`
	TaskID                      string         `json:"task_id,omitempty"`
	TaskKind                    string         `json:"task_kind,omitempty"`
	AssigneeRole                string         `json:"assignee_role,omitempty"`
	AssigneeAgentID             string         `json:"assignee_agent_id,omitempty"`
	Goal                        string         `json:"goal,omitempty"`
	InputContext                map[string]any `json:"input_context,omitempty"`
	Result                      map[string]any `json:"result,omitempty"`
	ParentTaskID                string         `json:"parent_task_id,omitempty"`
}

type InvToolOutcome struct {
	ChatID                  string
	Ok                      bool
	Op                      string
	Error                   string
	IncidentNumber          int64
	IncidentInvestigationID string
}

type promotedIncidentOutcome struct {
	IncidentNumber          int64
	IncidentInvestigationID string
}

type AgentToolExecutor struct {
	alertInvestigationStore    store.AlertInvestigationStore
	threadStore                store.InvestigationThreadStore
	mmClient                   *mattermost.Client
	slackClient                *slack.Client
	chatRouter                 *webhook.ChatRouter
	chatSync                   *ChatSyncService
	agentDMStore               store.AgentDMStore
	alertSideEffects           *AgentAlertSideEffects
	sseBroker                  *sse.Broker
	ssePublisher               *sse.DualPublisher
	vkClient                   *valkey.Client
	auditStore                 store.AuditStore
	investigationForwarder     webhook.InvestigationAgentForwarder
	pendingNotifier            pendingNotifier
	memoryExtractor            memoryExtractor
	chatSyncSem                chan struct{}
	notificationStore          store.NotificationStore
	userStore                  store.UserStore
	triageResultStore          store.TriageResultStore
	incidentStore              store.IncidentStore
	incidentCoordinationStore  store.IncidentCoordinationStore
	incidentInvestigationStore store.IncidentInvestigationStore
	postmortemStore            store.PostMortemStore
	serviceStore               store.ServiceStore
	escalationStore            store.EscalationStore
	onCallResolver             *oncall.Resolver
	icsRoleStore               store.ICSRoleStore
	incidentDocumentStore      store.IncidentDocumentStore
	escalationPublisher        *rabbitmq.Publisher
	lifecycle                  AlertInvestigationLifecycle

	// Cross-domain helpers injected from package api to avoid importing it.
	// Each is a thin closure over the package-api pure function of the same
	// name; nil means "not wired" and the call site preserves the legacy
	// skip/no-op behavior. Consolidated when api/incident is extracted.
	runAlertCascadeFn           func(ctx context.Context, alertStore store.Store, auditStore store.AuditStore, publisher *sse.DualPublisher, incidentNumber int64, agentID uuid.UUID, agentName string) store.AlertCascadeResult
	buildPostMortemDraftFn      func(ctx context.Context, documentStore store.IncidentDocumentStore, coordinationStore store.IncidentCoordinationStore, incidentStore store.IncidentStore, alertStore store.Store, inc *store.IncidentRecord, summary string) *store.PostMortemRecord
	forwardCoordinationUpdateFn func(ctx context.Context, incidentNumber int64, messageText string, mentions []string, agentRec *store.AgentTokenRecord)
	postIncidentResolveFn       func(ctx context.Context, incidentNumber int64)
}

type AgentAlertSideEffects struct {
	Store    store.Store
	Publish  func(*store.AlertRecord)
	Dedup    webhook.DedupCache
	SyncChat func(context.Context, *store.AlertRecord)
}

func NewAgentToolExecutor(
	alertInvestigationStore store.AlertInvestigationStore,
	mmClient *mattermost.Client,
	slackClient *slack.Client,
	agentDMStore store.AgentDMStore,
	chatRouter *webhook.ChatRouter,
) *AgentToolExecutor {
	return &AgentToolExecutor{
		alertInvestigationStore: alertInvestigationStore,
		mmClient:                mmClient,
		slackClient:             slackClient,
		chatRouter:              chatRouter,
		chatSync:                NewChatSyncService(mmClient, slackClient, alertInvestigationStore),
		agentDMStore:            agentDMStore,
		chatSyncSem:             make(chan struct{}, 16),
	}
}

func (e *AgentToolExecutor) requireCapability(agent agentTokenContext, cap string) error {
	if !capability.Has(agent.Capabilities, cap) {
		return fmt.Errorf("agent %q lacks required capability %q", agent.Name, cap)
	}
	return nil
}

func (e *AgentToolExecutor) requireAnyCapability(agent agentTokenContext, caps ...string) error {
	for _, cap := range caps {
		if capability.Has(agent.Capabilities, cap) {
			return nil
		}
	}
	return fmt.Errorf("agent %q lacks required capability", agent.Name)
}

func (e *AgentToolExecutor) extractMemories(inv *store.AlertInvestigationRecord) {
	if e.memoryExtractor == nil || inv == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("goroutine panic recovered", "panic", r, "location", "extract-memories")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		e.chatSyncSem <- struct{}{}
		defer func() { <-e.chatSyncSem }()
		_ = e.memoryExtractor.ExtractFromInvestigation(ctx, inv)
	}()
}

func (e *AgentToolExecutor) ExecuteInvTool(ctx context.Context, agentRec *store.AgentTokenRecord, cmd InvTool) InvToolOutcome {
	agent := agentTokenContext{ID: agentRec.ID, Name: agentRec.Name, AgentType: agentRec.AgentType, Capabilities: agentRec.Capabilities}
	chatID := cmd.ChatID
	op := strings.TrimSpace(strings.ToLower(cmd.Op))
	if chatID == "" {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "missing chat_id"}
	}
	if op == "" {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "missing op"}
	}

	if strings.HasPrefix(chatID, "incident_") && isIncidentToolOp(op) {
		agent := agentTokenContext{ID: agentRec.ID, Name: agentRec.Name, AgentType: agentRec.AgentType, Capabilities: agentRec.Capabilities}
		return e.executeIncidentTool(ctx, agentRec, agent, chatID, cmd, op)
	}

	if strings.HasPrefix(chatID, "incident_") && (op == "resolve_alert" || op == "reopen_alert") {
		incNumber, ok := incidentNumberFromIncidentChatID(chatID)
		if !ok {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "invalid incident chat_id"}
		}
		inc, err := e.incidentStore.GetIncident(ctx, incNumber)
		if err != nil || inc == nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "incident not found"}
		}
		roles := e.activeAgentIncidentRoles(ctx, agentRec.ID, incNumber)
		if !roles[string(ics.RoleIncidentCommander)] {
			return InvToolOutcome{
				ChatID: chatID,
				Ok:     false,
				Op:     op,
				Error:  "only the incident commander is authorized to resolve or reopen alerts associated with an active incident; responders must let the commander close the alert as part of incident resolution",
			}
		}
		invs, err := e.alertInvestigationStore.ListAlertInvestigations(ctx, map[string]any{"promoted_incident_id": inc.ID.String()})
		if err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "failed to list alert investigations for incident"}
		}
		if len(invs) == 0 {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "no linked alerts found for this incident"}
		}
		var errors []string
		for _, inv := range invs {
			res := e.executeAlertOwnerTool(ctx, agentRec, fmt.Sprintf("alert_%d", inv.PrimaryAlertNumber), strconv.FormatInt(inv.PrimaryAlertNumber, 10), cmd, op)
			if !res.Ok {
				errors = append(errors, res.Error)
			}
		}
		if len(errors) > 0 {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: strings.Join(errors, "; ")}
		}
		return InvToolOutcome{ChatID: chatID, Ok: true, Op: op}
	}

	ownerType, ownerID := parseOwnerFromChatID(chatID)
	if ownerType != store.ThreadOwnerAlert {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "unsupported chat_id format"}
	}
	if op == "resolve_alert" || op == "reopen_alert" {
		return e.executeAlertOwnerTool(ctx, agentRec, chatID, ownerID, cmd, op)
	}

	alertNumber, parseErr := strconv.ParseInt(ownerID, 10, 64)
	if parseErr != nil || alertNumber <= 0 {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "invalid alert number"}
	}
	inv, getErr := e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(ctx, alertNumber)
	if getErr != nil {
		logger.ErrorCtx(ctx, "inv_tool: get alert investigation", "error", getErr)
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "failed to load investigation"}
	}

	if inv == nil {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "investigation not found"}
	}

	investigationID := inv.AlertInvestigationID
	investigationUUID := inv.ID.String()

	if inv.AgentID != agentRec.ID.String() {
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "not assigned to this investigation"}
	}

	actor := agentActor(agentRec)

	if inv.Status == "assigned" {
		if err := e.alertInvestigationStore.TransitionAlertInvestigationStatus(ctx, investigationUUID, []string{"assigned"}, "investigating"); err != nil {
			logger.WarnCtx(ctx, "inv_tool: assigned→investigating transition failed", "investigation_id", investigationID, "error", err)
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "investigation status conflict, will be rescheduled"}
		}
		inv.Status = "investigating"
		e.publishInvestigationStatusChange(investigationID, "investigating")
	}
	switch op {
	case "resolve_alert", "reopen_alert":
		if e.alertSideEffects == nil || e.alertSideEffects.Store == nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "alert store not configured"}
		}
		fp, err := investigationFingerprint(inv, cmd.Fingerprint)
		if err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}

		if inv.PromotedIncidentID != nil && e.incidentStore != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "alert tools cannot be accessed from an incident investigation thread; once promoted, the responder must resolve alerts from the alert thread directly"}
		}

		var humanMsg string
		alertLabel, _ := resolveAlertLabel(e.alertSideEffects.Store, fp)

		switch op {
		case "resolve_alert":
			existing, _ := e.alertSideEffects.Store.GetByFingerprint(fp)
			alreadyResolved := existing != nil && existing.Status == "resolved"

			if !alreadyResolved {
				err := e.alertSideEffects.Store.ResolveAlertByUser(fp, actor)
				if err != nil {
					logger.WarnCtx(ctx, "Agent resolve_alert: alert resolve failed", "fingerprint", fp, "error", err)
				}
				if e.alertSideEffects.Dedup != nil {
					e.alertSideEffects.Dedup.RemoveTracking(ctx, fp)
				}
			}

			if cmd.RootCause != nil || cmd.Resolution != nil {
				_ = e.alertInvestigationStore.PatchAlertInvestigationOutcome(ctx, investigationUUID, cmd.RootCause, cmd.Resolution)
			}

			if e.allInvestigationAlertsResolved(inv) {
				targetStatus := store.AlertInvestigationStatusComplete
				if inv.PromotedIncidentID != nil {
					// A promoted alert investigation terminates as promoted;
					// incident-side completion bookkeeping runs inside
					// finalizeAlertInvestigation.
					targetStatus = store.AlertInvestigationStatusPromoted
				}
				if err := e.finalizeAlertInvestigation(ctx, inv, investigationID, targetStatus, actor.Username, agentRec.ID); err != nil {
					logger.WarnCtx(ctx, "inv_tool: resolve_alert finalization failed", "investigation_id", investigationID, "target", targetStatus, "error", err)
					return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "investigation status conflict"}
				}
			}

			humanMsg = fmt.Sprintf("✅ *%s* resolved alert %s", actor.Username, alertLabel)
		case "reopen_alert":
			ev := store.AlertEventWithActor("reopened", time.Now(), actor)
			if err := e.alertSideEffects.Store.ReopenAlert(fp, ev); err != nil {
				return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
			}
			if e.alertSideEffects.Dedup != nil {
				_ = e.alertSideEffects.Dedup.MarkTracked(ctx, fp)
			}
			if store.IsReopenableInvestigationStatus(inv.Status) {
				agentAvailable := inv.AgentID != "" && e.investigationForwarder != nil && e.investigationForwarder.AgentOnline(inv.AgentID)
				if agentAvailable {
					if err := e.alertInvestigationStore.TransitionAlertInvestigationStatus(ctx, investigationUUID, slices.Concat(store.InvestigationTerminalStatuses, []string{"paused"}), "investigating"); err != nil {
						logger.WarnCtx(ctx, "inv_tool: reopen transition to investigating failed", "investigation_id", investigationID, "error", err)
					} else {
						e.publishInvestigationStatusChange(investigationID, "investigating")
					}
					event := sse.Event{
						Type: "investigation_resume",
						Data: map[string]any{
							"alert_investigation_id": investigationID,
							"reason":                 fmt.Sprintf("reopened alert %s", fp),
							"actor":                  actor.Username,
						},
					}
					if err := e.investigationForwarder.ForwardEventToAgent(inv.AgentID, event); err != nil {
						logger.ErrorCtx(ctx, "Failed to forward investigation_resume for investigation on agent tool reopen", "investigation_id", investigationID, "error", err)
					}
				} else {
					previousAgentName := inv.AgentName
					if err := e.alertInvestigationStore.UpdateAlertInvestigationAgent(ctx, investigationUUID, "", "", ""); err != nil {
						logger.ErrorCtx(ctx, "Failed to redelegate investigation on agent reopen", "investigation_id", investigationID, "error", err)
					} else {
						reassignMsg := fmt.Sprintf("Investigation reassigned: reopened alert %s", fp)
						if previousAgentName != "" {
							reassignMsg = fmt.Sprintf("Investigation reassigned: agent %s unavailable, reopened alert %s", previousAgentName, fp)
						}
						_ = e.alertInvestigationStore.AddAlertInvestigationUpdate(ctx, investigationID, store.InvestigationUpdate{
							Type:    store.UpdateTypeComment,
							Message: reassignMsg,
							Source:  store.UpdateSourceSystem,
						})
						if e.pendingNotifier != nil {
							e.pendingNotifier.NotifyPending()
						}
						e.publishInvestigationStatusChange(investigationID, "pending")
					}
				}
			}
			humanMsg = fmt.Sprintf("🔄 *%s* reopened alert %s", actor.Username, alertLabel)
		}
		e.afterAlertMutation(ctx, fp, actor)
		e.postCommandUpdate(ctx, investigationID, inv, humanMsg, actor)
		e.logAudit(op, actor.Username, investigationID, fp)

		if ownerType != "" && ownerID != "" {
			e.ensureThreadMessage(ctx, ownerType, ownerID, "action", humanMsg)
		}

		return InvToolOutcome{ChatID: chatID, Ok: true, Op: op, Error: ""}

	case "set_outcome":
		if cmd.RootCause == nil && cmd.Resolution == nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "provide root_cause and/or resolution"}
		}
		if err := e.alertInvestigationStore.PatchAlertInvestigationOutcome(ctx, investigationUUID, cmd.RootCause, cmd.Resolution); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
		var humanMsg string
		if cmd.RootCause != nil && strings.TrimSpace(*cmd.RootCause) != "" {
			humanMsg = fmt.Sprintf("🔍 *%s* set root cause: _%s_", actor.Username, *cmd.RootCause)
		} else if cmd.Resolution != nil && strings.TrimSpace(*cmd.Resolution) != "" {
			humanMsg = fmt.Sprintf("🔧 *%s* set resolution: _%s_", actor.Username, *cmd.Resolution)
		}
		if humanMsg != "" {
			e.postCommandUpdate(ctx, investigationID, inv, humanMsg, actor)
		}
		if inv.PromotedIncidentID != nil {
			if inc, err := e.incidentStore.GetIncidentByID(ctx, *inv.PromotedIncidentID); err == nil && inc != nil {
				e.updateIncidentFromOutcome(ctx, inc.IncidentNumber, investigationID, cmd.RootCause, cmd.Resolution)
			}
		}
		e.publishInvestigationPatch(investigationID)
		e.logAudit("set_outcome", actor.Username, investigationID, "")

		if ownerType != "" && ownerID != "" {
			e.ensureThreadMessage(ctx, ownerType, ownerID, "resolution", humanMsg)
		}

		return InvToolOutcome{ChatID: chatID, Ok: true, Op: op, Error: ""}

	case "cancel_investigation":
		if inv.Status != "investigating" && inv.Status != "assigned" {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "can only cancel active investigations"}
		}
		if e.alertSideEffects != nil && e.alertSideEffects.Store != nil {
			for _, a := range inv.Alerts {
				existing, err := e.alertSideEffects.Store.GetByFingerprint(a.Fingerprint)
				if err != nil {
					logger.WarnCtx(ctx, "cancel_investigation: lookup failed", "fingerprint", a.Fingerprint, "error", err)
					continue
				}
				if existing == nil || existing.Status == "resolved" {
					continue
				}
				if err := e.alertSideEffects.Store.ResolveAlertByUser(a.Fingerprint, actor); err != nil {
					logger.WarnCtx(ctx, "cancel_investigation: resolve failed", "fingerprint", a.Fingerprint, "error", err)
					continue
				}
				if e.alertSideEffects.Dedup != nil {
					e.alertSideEffects.Dedup.RemoveTracking(ctx, a.Fingerprint)
				}
				e.afterAlertMutation(ctx, a.Fingerprint, actor)
			}
		}
		if err := e.alertInvestigationStore.TransitionAlertInvestigationStatus(ctx, investigationUUID, []string{"investigating", "assigned"}, "cancelled"); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
		inv.Status = "cancelled"
		e.extractMemories(inv)
		humanMsg := fmt.Sprintf("🚫 *%s* cancelled the investigation", actor.Username)
		if cmd.Reason != "" {
			humanMsg += fmt.Sprintf(": %s", cmd.Reason)
		}
		e.postCommandUpdate(ctx, investigationID, inv, humanMsg, actor)
		e.publishInvestigationEvent(investigationID, "investigation_complete", map[string]any{
			"alert_investigation_id": investigationID,
			"status":                 "cancelled",
		})
		e.logAudit("cancel_investigation", actor.Username, investigationID, "")

		if ownerType != "" && ownerID != "" {
			e.ensureThreadMessage(ctx, ownerType, ownerID, "action", humanMsg)
		}

		return InvToolOutcome{ChatID: chatID, Ok: true, Op: op, Error: ""}

	case "pause_investigation":
		if inv.Status != "investigating" {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "can only pause active investigations"}
		}
		if err := e.alertInvestigationStore.TransitionAlertInvestigationStatus(ctx, investigationUUID, []string{"investigating"}, "paused"); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
		inv.Status = "paused"
		humanMsg := fmt.Sprintf("⏸️ *%s* paused the investigation", actor.Username)
		if cmd.Reason != "" {
			humanMsg += fmt.Sprintf(": %s", cmd.Reason)
		}
		e.postCommandUpdate(ctx, investigationID, inv, humanMsg, actor)
		e.publishInvestigationEvent(investigationID, "investigation_status_changed", map[string]any{
			"alert_investigation_id": investigationID,
			"status":                 "paused",
		})
		e.logAudit("pause_investigation", actor.Username, investigationID, "")

		if ownerType != "" && ownerID != "" {
			e.ensureThreadMessage(ctx, ownerType, ownerID, "action", humanMsg)
		}

		return InvToolOutcome{ChatID: chatID, Ok: true, Op: op, Error: ""}

	case "promote_to_incident":
		promo, err := e.performPromoteToIncident(ctx, agentRec, agent, chatID, inv, cmd)
		if err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
		humanMsg := fmt.Sprintf("🚨 *%s* promoted the alert investigation to incident [**#%d**](/incidents/%d). The incident will be investigated by the incident response team in its own investigation thread.", actor.Username, promo.IncidentNumber, promo.IncidentNumber)
		e.postCommandUpdate(ctx, investigationID, inv, humanMsg, actor)
		e.publishInvestigationStatusChange(investigationID, "promoted")
		e.logAudit("promote_to_incident", actor.Username, investigationID, "")

		if ownerType != "" && ownerID != "" {
			e.ensureThreadMessage(ctx, ownerType, ownerID, "action", humanMsg)
		}

		return InvToolOutcome{
			ChatID:                  chatID,
			Ok:                      true,
			Op:                      op,
			IncidentNumber:          promo.IncidentNumber,
			IncidentInvestigationID: promo.IncidentInvestigationID,
		}

	case "triage_feedback":
		triageID := cmd.TriageResultID
		if triageID == "" {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "triage_result_id is required for triage_feedback"}
		}

		triage, err := e.triageResultStore.Get(ctx, triageID)
		if err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: fmt.Sprintf("get triage result: %v", err)}
		}
		if triage == nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "triage result not found: " + triageID}
		}

		patch := &store.TriageResultRecord{}
		if cmd.Agreed {
			patch.Outcome = store.TriageResultOutcomeConfirmed
		} else {
			patch.Outcome = store.TriageResultOutcomeOverridden
			if cmd.CorrectDecision != "" {
				patch.OverriddenTo = cmd.CorrectDecision
			}
			if cmd.CorrectSeverity != "" {
				patch.SeverityClassified = cmd.CorrectSeverity
			}
		}
		_, err = e.triageResultStore.Update(ctx, triageID, patch)
		if err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: fmt.Sprintf("update triage result: %v", err)}
		}
		return InvToolOutcome{ChatID: chatID, Ok: true, Op: op}

	case "assign_investigation":
		if err := e.requireCapability(agent, capability.Command); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
		targetAgentID := cmd.TargetAgentID
		if targetAgentID == "" {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "target_agent_id is required"}
		}
		var (
			inv *store.AlertInvestigationRecord
			err error
		)
		if !strings.HasPrefix(chatID, "alert_") {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "chat_id must be alert_<number>"}
		}
		alertNumStr := strings.TrimPrefix(chatID, "alert_")
		alertNumber, parseErr := strconv.ParseInt(alertNumStr, 10, 64)
		if parseErr != nil || alertNumber <= 0 {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "invalid alert number in chat_id"}
		}
		inv, err = e.alertInvestigationStore.GetCurrentAlertInvestigationByAlertNumber(ctx, alertNumber)
		if err != nil || inv == nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "investigation not found"}
		}
		invID := inv.AlertInvestigationID
		if inv.AgentID == "" {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "investigation has no assigned agent"}
		}
		if inv.AgentID != agentRec.ID.String() {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "only the assigned agent can reassign"}
		}
		if err := e.alertInvestigationStore.UpdateAlertInvestigationAgent(ctx, inv.ID.String(), targetAgentID, "", ""); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: fmt.Sprintf("reassign: %v", err)}
		}
		if e.investigationForwarder != nil {
			prompt := fmt.Sprintf("Investigation %s reassigned from agent %s.", invID, agentRec.Name)
			_ = e.investigationForwarder.ForwardToAgent(targetAgentID, invID, "system", "System", prompt)
		}
		if e.auditStore != nil {
			e.auditStore.Log(store.AuditInvestigationUpdated, nil, agentRec.Name, "", "", true, map[string]any{
				"alert_investigation_id": invID,
				"action":                 "reassigned",
				"from_agent":             agentRec.ID.String(),
				"to_agent":               targetAgentID,
			})
		}
		return InvToolOutcome{ChatID: chatID, Ok: true, Op: op}

	case "set_incident_priority", "set_incident_severity", "trigger_escalation", "mitigate_incident", "resolve_incident", "begin_triage", "promote_incident", "assign_incident_role":
		incidentNumber := cmd.IncidentNumber
		if incidentNumber == 0 && inv.PromotedIncidentID != nil && e.incidentStore != nil {
			if inc, err := e.incidentStore.GetIncidentByID(ctx, *inv.PromotedIncidentID); err == nil && inc != nil {
				incidentNumber = inc.IncidentNumber
			}
		}
		if incidentNumber == 0 {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "investigation not promoted to incident, or incident_number is required"}
		}
		if _, err := e.incidentStore.GetIncident(ctx, incidentNumber); err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: fmt.Sprintf("incident %d not found", incidentNumber)}
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
		}
		if err != nil {
			return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: err.Error()}
		}
		return InvToolOutcome{ChatID: chatID, Ok: true, Op: op}

	default:
		return InvToolOutcome{ChatID: chatID, Ok: false, Op: op, Error: "unknown op"}
	}
}

func isIncidentToolOp(op string) bool {
	switch op {
	case "set_incident_priority", "set_incident_severity", "trigger_escalation", "mitigate_incident", "resolve_incident", "begin_triage", "promote_incident", "assign_incident_role", "pause_investigation", "cancel_investigation", "post_handoff", "publish_status_update", "set_incident_resolution_docs":
		return true
	default:
		return false
	}
}

func mapToolAuditEvent(op string) store.AuditEvent {
	switch op {
	case "resolve_alert":
		return store.AuditAlertResolved
	case "reopen_alert":
		return store.AuditAlertReopened
	case "cancel_investigation", "pause_investigation":
		return store.AuditInvestigationUpdated
	default:
		return store.AuditInvestigationUpdated
	}
}
