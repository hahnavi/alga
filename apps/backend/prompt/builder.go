package prompt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"alga/incident"
	"alga/knowledge"
	"alga/rabbitmq"
	"alga/routing"
	"alga/store"
)

type KnowledgeSource interface {
	BuildContext(ctx context.Context, inv *store.AlertInvestigationRecord) *knowledge.Context
}

type DispatchInput struct {
	InvestigationID      string
	InvestigationTimeout time.Duration
	Alerts               []rabbitmq.CorrelatedAlert
	Severity             string
	ImpactLevel          string
	Priority             string
	CorrelationKey       string
	AdminTeamID          string
	AdminTeamName        string
	IncidentID           string
	IncidentNumber       int64
	IncidentScope        bool
	IncidentRole         string
	// IncidentStatus is the current lifecycle status of the incident (detected,
	// triaging, active, mitigated, resolved, closed). The commander prompt uses
	// it to inject a phase-aware checklist so the orchestrator knows what to do
	// next without a separate playbook entity. Empty when not incident-scoped.
	IncidentStatus                  string
	PromotedAlertHandoff            bool
	PromotedIncidentInvestigationID string
	PrimaryAlertFingerprint         string
	PrimaryAlertNumber              int64
	ServiceOwner                    string
	RunbookURL                      string
	SuggestedActions                []string
	SimilarInvestigationIDs         []string
}

func FromAlertInvestigationRecord(rec *store.AlertInvestigationRecord) DispatchInput {
	in := DispatchInput{
		InvestigationID:         rec.AlertInvestigationID,
		Alerts:                  rec.Alerts,
		Severity:                rabbitmq.DetermineAlertSeverity(rec.Alerts),
		CorrelationKey:          rec.CorrelationKey,
		PrimaryAlertFingerprint: rec.PrimaryAlertFingerprint,
		PrimaryAlertNumber:      rec.PrimaryAlertNumber,
	}
	applyStoredTriageEnrichment(&in, rec.TriageEnrichment)
	if rec.PromotedIncidentID != nil {
		in.IncidentScope = true
		in.IncidentID = rec.PromotedIncidentID.String()
		in.PromotedAlertHandoff = true
	}
	if rec.PromotedIncidentInvestigationID != nil {
		in.PromotedIncidentInvestigationID = rec.PromotedIncidentInvestigationID.String()
	}
	return in
}

func FromInvestigateMessage(msg rabbitmq.InvestigateMessage) DispatchInput {
	return DispatchInput{
		InvestigationID:         msg.InvestigationID,
		Alerts:                  msg.Alerts,
		Severity:                msg.Severity,
		CorrelationKey:          msg.CorrelationKey,
		ServiceOwner:            msg.TriageEnrichment.ServiceOwner,
		RunbookURL:              msg.TriageEnrichment.RunbookURL,
		SuggestedActions:        msg.TriageEnrichment.SuggestedActions,
		SimilarInvestigationIDs: msg.TriageEnrichment.SimilarInvestigationIDs,
		PrimaryAlertFingerprint: msg.PrimaryAlertFingerprint,
		PrimaryAlertNumber:      msg.PrimaryAlertNumber,
	}
}

func BuildDispatchPrompt(in DispatchInput) string {
	var b strings.Builder

	b.WriteString("# Alert Investigation\n\n")
	b.WriteString("Investigate:")
	if in.PrimaryAlertNumber > 0 && in.PrimaryAlertFingerprint != "" {
		fmt.Fprintf(&b, " **Alert #%d** (%s)", in.PrimaryAlertNumber, in.PrimaryAlertFingerprint)
	} else if len(in.Alerts) > 0 {
		first := in.Alerts[0]
		alertName := first.Labels["alertname"]
		if alertName == "" {
			alertName = first.Fingerprint
		}
		if first.AlertNumber > 0 {
			fmt.Fprintf(&b, " **Alert #%d** %s", first.AlertNumber, alertName)
		} else {
			fmt.Fprintf(&b, " **%s**", alertName)
		}
	}
	if extra := len(in.Alerts) - 1; extra > 0 {
		fmt.Fprintf(&b, " + %d correlated alert%s", extra, pluralS(extra))
	}
	b.WriteString("\n\n")

	b.WriteString("## Investigation Context\n")
	if in.InvestigationID != "" {
		fmt.Fprintf(&b, "**Investigation ID:** %s\n", in.InvestigationID)
	}
	if toolThread := toolThreadChatID(in); toolThread != "" {
		fmt.Fprintf(&b, "**Tool thread:** %s — pass this value (NOT the Investigation ID above) as `investigation_id` to alga_* tools.\n", toolThread)
	}
	if in.Severity != "" {
		fmt.Fprintf(&b, "**Severity:** %s\n", in.Severity)
	}
	if in.ImpactLevel != "" {
		fmt.Fprintf(&b, "**Impact:** %s\n", in.ImpactLevel)
	}
	if in.Priority != "" {
		fmt.Fprintf(&b, "**Priority:** %s\n", in.Priority)
	}
	fmt.Fprintf(&b, "**Alerts:** %d\n", len(in.Alerts))
	if in.CorrelationKey != "" {
		fmt.Fprintf(&b, "**Correlation:** %s\n", in.CorrelationKey)
	}
	if in.InvestigationTimeout > 0 {
		fmt.Fprintf(&b, "**Time budget:** Complete, pause, or escalate within %s.\n", in.InvestigationTimeout.String())
	}
	fmt.Fprintf(&b, "**Depth:** %s\n", investigationDepthHint(in.Severity))
	b.WriteString("\n")

	if in.ServiceOwner != "" || in.RunbookURL != "" || len(in.SuggestedActions) > 0 || len(in.SimilarInvestigationIDs) > 0 {
		b.WriteString("## Triage Enrichment\n")
		if in.ServiceOwner != "" {
			fmt.Fprintf(&b, "**Service owner:** %s\n", in.ServiceOwner)
		}
		if in.RunbookURL != "" {
			fmt.Fprintf(&b, "**Runbook:** %s\n", in.RunbookURL)
		}
		if len(in.SuggestedActions) > 0 {
			b.WriteString("**Suggested actions:**\n")
			for _, action := range in.SuggestedActions {
				fmt.Fprintf(&b, "- %s\n", action)
			}
		}
		if len(in.SimilarInvestigationIDs) > 0 {
			fmt.Fprintf(&b, "**Similar investigations:** %s\n", strings.Join(in.SimilarInvestigationIDs, ", "))
		}
		b.WriteString("\n")
	}

	labelMaps := make([]map[string]string, len(in.Alerts))
	for i, a := range in.Alerts {
		labelMaps[i] = a.Labels
	}
	commonLabels := routing.FindCommonKeyValues(labelMaps)
	if len(commonLabels) > 0 {
		fmt.Fprintf(&b, "## Shared Labels\n")
		for k, v := range commonLabels {
			fmt.Fprintf(&b, "- %s: %s\n", k, v)
		}
		fmt.Fprintf(&b, "\n")
	}

	b.WriteString("## Alert Details\n")
	maxAlerts := alertDetailLimit(in.Severity, len(in.Alerts))
	for i, a := range in.Alerts {
		if i >= maxAlerts {
			fmt.Fprintf(&b, "... and %d more\n", len(in.Alerts)-i)
			break
		}
		alertName := a.Labels["alertname"]
		if alertName == "" {
			alertName = a.Fingerprint
		}
		if a.AlertNumber > 0 {
			fmt.Fprintf(&b, "### %d. #%d %s\n", i+1, a.AlertNumber, alertName)
		} else {
			fmt.Fprintf(&b, "### %d. %s\n", i+1, alertName)
		}
		if a.AlertNumber > 0 {
			fmt.Fprintf(&b, "**Alert ID:** #%d\n", a.AlertNumber)
		}
		fmt.Fprintf(&b, "**Status:** %s\n", a.Status)
		if a.StartsAt != "" {
			fmt.Fprintf(&b, "**Firing since:** %s\n", a.StartsAt)
		}
		if summary, ok := a.Annotations["summary"]; ok && summary != "" {
			fmt.Fprintf(&b, "**Summary:** %s\n", summary)
		}
		if desc, ok := a.Annotations["description"]; ok && desc != "" {
			fmt.Fprintf(&b, "**Description:** %s\n", desc)
		}
		if len(a.Values) > 0 {
			fmt.Fprintf(&b, "**Values:** ")
			first := true
			for k, v := range a.Values {
				if !first {
					fmt.Fprintf(&b, ", ")
				}
				fmt.Fprintf(&b, "%s=%.2f", k, v)
				first = false
			}
			fmt.Fprintf(&b, "\n")
		}
		diffLabels := diffLabels(a.Labels, commonLabels)
		if len(diffLabels) > 0 {
			fmt.Fprintf(&b, "**Labels:**\n")
			for k, v := range diffLabels {
				fmt.Fprintf(&b, "- %s: %s\n", k, v)
			}
		}
		if runbookURL, ok := a.Annotations["runbook_url"]; ok && runbookURL != "" {
			fmt.Fprintf(&b, "**Runbook:** %s\n", runbookURL)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(in.Alerts) > 0 && in.Alerts[0].GeneratorURL != "" {
		b.WriteString("## Links\n")
		fmt.Fprintf(&b, "[View in Grafana](%s)\n", in.Alerts[0].GeneratorURL)
	}

	if guidance := serviceDownGuidance(in.Alerts, in.IncidentScope); guidance != "" {
		fmt.Fprintf(&b, "\n%s\n", guidance)
	}

	writeInvestigationInstructions(&b, in, true)
	writeAvailableTools(&b, in)
	writeIncidentInstructions(&b, in)
	writeHumanEscalation(&b, in)

	return b.String()
}

// BuildDispatchSystemContext returns only the behavioral-rule sections of the
// dispatch prompt (investigation instructions, available tools, incident
// instructions, human escalation). Agents that support system-prompt injection
// (e.g. the native alga-agent) place this in the LLM system message so the
// rules persist across long tool-heavy conversations. The full dispatch prompt
// returned by BuildDispatchPrompt still contains these sections verbatim for
// agents that only read the user message.
func BuildDispatchSystemContext(in DispatchInput) string {
	var b strings.Builder

	writeInvestigationInstructions(&b, in, false)
	writeAvailableTools(&b, in)
	writeIncidentInstructions(&b, in)
	writeHumanEscalation(&b, in)

	return b.String()
}

func writeInvestigationInstructions(b *strings.Builder, in DispatchInput, leadingNL bool) {
	if leadingNL {
		b.WriteString("\n")
	}
	b.WriteString("## Investigation Instructions\n")
	fmt.Fprintf(b, "**Alerts are pre-acknowledged by the system.** Do NOT send a chat text message saying \"acknowledging\" or \"resolved\". The system posts action messages automatically.\n")
	if !in.IncidentScope {
		fmt.Fprintf(b, "**Verify before acting, then follow the runbook for promotion.** Do not blindly promote. First verify the alert is genuine and still firing (re-check the linked alerts with `alga_list_alerts`; if every linked alert is already `resolved`, finalize the investigation with `alga_set_outcome` instead of promoting). The alert runbook is authoritative for promotion when it speaks: if it specifies promotion criteria — including mandatory/immediate promotion (e.g. \"any node down → promote immediately\") — call `alga_promote_to_incident` as the runbook directs once its conditions are met, without mitigating first or imposing extra gates. **A runbook is not required for promotion.** When the runbook is silent on promotion — or no runbook matches this alert at all — promote when there is real, current **user-facing** impact AND the alert is still firing. Verify the user-facing impact (failing requests, error budget burn, customer-visible errors, blocked deploys) and call `alga_promote_to_incident`. **Once you call `alga_promote_to_incident`, stop immediately — do not run any further commands, SSH sessions, log checks, or tool calls in this investigation. The incident response team owns all follow-up from that point; your alert investigation is done.**\n")
	}
	if in.IncidentScope && !in.PromotedAlertHandoff {
		if in.IncidentRole == "incident_commander" {
			fmt.Fprintf(b, "**Verify recovery before resolving.** You are strictly forbidden from performing technical investigation, environment validation, running commands against the environment, or inspecting environment/service health. Closing the linked alert with `alga_resolve_alert` (after checking its state with `alga_list_alerts`) is part of incident closure, not a technical action, and is part of your role. All other technical actions must be handled by the Responder.\n")
		} else {
			fmt.Fprintf(b, "**Verify recovery before resolving.** Do not resolve based only on process state, TCP readiness, or a single green check. First verify the user-visible service or database operation that the alert describes. Once recovery is verified, do NOT attempt to call alert tools (such as alert resolution/reopening) or resolve the alert yourself, as you are FORBIDDEN from calling `alga_resolve_alert` or `alga_reopen_alert` on alerts linked to an active incident — alert closure is part of incident closure and is owned by the incident commander. Let the commander close the alert. You are also FORBIDDEN from calling `alga_who_is_on_call` to identify who to hand off to. You do not need to look up who is on call; the handoff in the investigation thread (using `alga_post_handoff` with `audience=\"commander\"`) is always directly to the incident commander. Record findings (root cause, evidence, impact, resolution) on the incident investigation thread via coordination updates.\n")
		}
	} else if !in.IncidentScope {
		fmt.Fprintf(b, "**Verify recovery before resolving.** Do not resolve based only on process state, TCP readiness, or a single green check. First verify the user-visible service or database operation that the alert describes. After recovery is verified (or the alert is already recovered), call `alga_resolve_alert` to resolve the alert and finalize the investigation. Record root cause, evidence, impact, and resolution via `alga_set_outcome`; include `root_cause` and `resolution` in the resolve call only when they are ready.\n")
	}
	fmt.Fprintf(b, "**Read runbooks before debugging.** Knowledge search results and the auto-injected shared-knowledge previews are truncated. When a runbook or known issue matches this alert, call `alga_get_knowledge` with its id to read the full body and follow its steps before attempting your own recovery commands. The full body often holds exact users, commands, and ports that the preview omits.\n")
}

func writeAvailableTools(b *strings.Builder, in DispatchInput) {
	b.WriteString("\n## Available Tools\n")
	if in.IncidentScope && !in.PromotedAlertHandoff {
		commonTools := "`alga_publish_status_update`, `alga_get_incident_context`, `alga_cancel_investigation`, `alga_pause_investigation`, `alga_set_severity`, `alga_search_knowledge`, `alga_get_knowledge`, `alga_create_knowledge`, `alga_list_alerts`"
		if in.IncidentRole == "responder" {
			fmt.Fprintf(b, "%s. Role-specific tools (commander-only or responder-only) are detailed in the Incident Instructions section below.\n", commonTools)
		} else {
			fmt.Fprintf(b, "%s, `alga_post_handoff`, `alga_resolve_alert`. Role-specific tools (commander-only or responder-only) are detailed in the Incident Instructions section below.\n", commonTools)
		}
	} else if in.IncidentScope {
		fmt.Fprintf(b, "`alga_cancel_investigation`, `alga_pause_investigation`, `alga_set_severity`, `alga_search_knowledge`, `alga_get_knowledge`, `alga_create_knowledge`, `alga_list_alerts`.\n")
	} else {
		fmt.Fprintf(b, "`alga_cancel_investigation`, `alga_pause_investigation`, `alga_set_outcome`, `alga_resolve_alert`, `alga_reopen_alert`, `alga_set_severity`, `alga_search_knowledge`, `alga_get_knowledge`, `alga_create_knowledge`, `alga_list_alerts`.\n")
	}
}

func writeIncidentInstructions(b *strings.Builder, in DispatchInput) {
	if in.IncidentScope {
		b.WriteString("\n## Incident Instructions\n")
		if in.PromotedAlertHandoff {
			incidentRef := ""
			if in.IncidentNumber > 0 {
				incidentRef = fmt.Sprintf(" Incident **#%d**", in.IncidentNumber)
			}
			fmt.Fprintf(b, "This alert investigation has been promoted to an incident%s. Your alert investigation is now complete — stop here and take no further investigative action.\n", incidentRef)
			b.WriteString("- Follow-up is handled by a separate incident investigation, owned by the incident-response team (commander, communicator, responders) in its own thread.\n")
			b.WriteString("- Do NOT call incident tools (priority, severity, mitigation, resolution, escalation, coordination updates, status updates) from this alert investigation — those belong to the incident team.\n")
			b.WriteString("- Do NOT post in the incident coordination thread; that thread is owned by the incident team.\n")
			b.WriteString("- Refer to the incident by its number only (for example \"Incident #26\"). Never mention, echo, or surface incident investigation IDs, UUIDs, or other internal identifiers in your messages — they are not user-facing or linkable.\n")
			b.WriteString("- If you have not already recorded the root cause and resolution, finalize this alert investigation with `alga_set_outcome`, then stop.\n")
			b.WriteString("- Do NOT suggest next steps, list tasks, write instructions, or tell the incident team or commander what to do (e.g., do NOT write a \"The incident team should:...\" or \"Next Steps:\" section). Your final concluding message in this chat thread must ONLY state that you have promoted the alert to the incident, reference the incident number, and provide the confirmed user-facing impact as context.\n")
			b.WriteString("- Do NOT list technical recovery steps, log checks, or command suggestions as \"Required Actions (for incident commander)\" in your final notes or summaries. The incident commander does not perform technical actions; technical recovery tasks belong only to the Responder.\n")
			b.WriteString("\n")
		} else {
			fmt.Fprintf(b, "%s\n", incidentRoleInstructions(in.IncidentRole, in.IncidentStatus))
		}
	}
}

func writeHumanEscalation(b *strings.Builder, in DispatchInput) {
	if in.AdminTeamID != "" {
		teamLabel := in.AdminTeamName
		if teamLabel == "" {
			teamLabel = "ops-team"
		}
		fmt.Fprintf(b, "\n## Human Escalation\nIf you need human confirmation, input, or approval during investigation, send a chat text message with `\"mentions\": [\"team:%s\"]` to notify the **%s** team, then call `alga_pause_investigation` to pause until a human responds. Example: post a text message asking your question with the ops team mentioned, then pause.\n", in.AdminTeamID, teamLabel)
	}
}

func BuildDispatchPromptWithKnowledge(ctx context.Context, in DispatchInput, src KnowledgeSource) string {
	base := BuildDispatchPrompt(in)
	if src == nil {
		return base
	}
	inv := &store.AlertInvestigationRecord{
		AlertInvestigationID: in.InvestigationID,
		Alerts:               in.Alerts,
		CorrelationKey:       in.CorrelationKey,
	}
	kctx := src.BuildContext(ctx, inv)
	if kctx == nil || kctx.IsEmpty() {
		return base
	}
	return base + kctx.PromptBlock()
}

func incidentRoleInstructions(role, incidentStatus string) string {
	var roleSpecific string
	switch role {
	case "incident_commander":
		roleSpecific = "**Your incident role: Incident Commander.** Own incident direction, escalation decisions, final calls, and documentation quality. Commander tools: `alga_set_incident_priority`, `alga_set_incident_severity`, `alga_trigger_escalation`, `alga_publish_status_update`, `alga_mitigate_incident`, `alga_resolve_incident`, `alga_begin_triage`, `alga_promote_incident`, `alga_assign_incident_role`, `alga_resolve_alert`, `alga_list_alerts`, `alga_dispatch_task`, `alga_synthesize_findings`. You publish status updates directly — do NOT route them through the Communications Lead (the old `alga_request_status_update` flow is removed; to ask the communicator to publish a status update, dispatch a task with kind=communicate, assignee_role=communicator, goal=\"Publish a status update at level X\"). **You are the incident orchestrator. You do NOT investigate directly. Decompose the incident into bounded tasks via `alga_dispatch_task` (kind=investigate for technical work, kind=communicate for status updates, kind=verify for verification, kind=mitigate for recovery actions). Target each task at a role (responder/communicator). Track task progress via the GET /api/v1/agent/incidents/{id}/tasks route (and advertise `alga_list_tasks`). When child investigations complete, synthesize their findings via `alga_synthesize_findings` and write the incident conclusion. You may still run commands yourself ONLY for non-delegable commander actions (priority/severity/escalation/resolution); all technical investigation and verification is delegated to responders.** **You are strictly forbidden from performing technical validation, running commands against the environment, or inspecting environment/service health. All technical work, commands, and validation must be handled by the Responder.** Alert state checks (`alga_list_alerts`) and closure (`alga_resolve_alert`) are part of incident closure, not technical actions, and are part of your role — but you do not perform verification of recovery yourself. **Commander verification is a paper review of the Responder's evidence, not a technical action.** When a Responder hands off with `Ready for commander verification.`, your job is to: (1) confirm the Responder's evidence is complete and consistent (recovery action, verification checks and their results, impact assessment) — the investigation summary, coordination messages, and responder updates together constitute the evidence; you do NOT require any formal findings tool calls as findings/outcomes are gathered directly from the coordination thread, (2) verify the linked alert's status with `alga_list_alerts` and, if it is still firing, close it with `alga_resolve_alert` (the Responder may have already done this from the alert owner thread; in that case `alga_list_alerts` will show `resolved` and no further action is needed), (3) publish a public-consumable status update with `status_level=\"resolved\"` directly via `alga_publish_status_update`, (4) decide resolution/closure, and (5) call `alga_resolve_incident`. **You are the sole role responsible for publishing the \"resolved\" status update; do not ask the responder to do it.** **You also own alert closure as part of incident closure — do not ask the Responder to resolve the alert.** Do NOT run any technical commands, ssh anywhere, restart anything, or re-execute the Responder's checks. If the evidence is genuinely absent or contradictory, post a coordination reply asking the Responder to clarify — do not run commands yourself. **Do NOT @mention the Responder or any other teammate in appreciation, acknowledgement, sign-off, or recap.** An @mention activates them and forces a ping-pong reply. When you have accepted the Responder's handoff, either act on it (verify and close the linked alert, publish the resolved status update, call `alga_resolve_incident`, set `alga_set_incident_resolution_docs`) or post a brief coordination reply without any mentions. A bare \"Great work, @Responder\" message is forbidden. **When resolving the incident, write a SHORT executive summary (3-6 sentences, roughly 100-150 words) in plain chronological prose for non-technical stakeholders: what broke and when, what was done about it, and the current state. It is a single narrative — NOT a structured report. Do NOT use ANY labels, prefixes, bold tags, or section headings; specifically forbidden forms include \"Root Cause:\", \"Trigger:\", \"Recovery:\", \"Resolution:\", \"Impact:\", \"Timeline:\", and \"Actions:\". Do NOT duplicate the technical detail that lives in the dedicated root_cause, resolution, impact_assessment, and actions_taken fields (file paths, exact commands, per-node state, log excerpts, timeline traces) — those are separate mandatory sections a reader reaches after the summary. If a detail is already covered by one of those sections, leave it out of the summary. Good shape: \"At 09:41 UTC a PostgreSQL replica stopped accepting connections and triggered an alert. The replica was reinitialized from the healthy primary while the cluster remained available. All nodes are now healthy with zero replication lag, and no customer impact was observed.\"**"
	case "communications_lead":
		roleSpecific = "**Your incident role: Communications Lead.** You are the Communications Lead. You will receive communicate-kind coordination tasks dispatched by the commander (these wake you via the coordination_task_dispatched event). When you receive such a task, execute it (e.g., publish the requested status update via `alga_publish_status_update`) and complete the task via `alga_complete_task` with a result describing what was published. You are NOT expected to take proactive action on incident progression beyond tasks explicitly dispatched to you; when no task is pending for you, wait quietly and do NOT post in the incident coordination thread in response to commander or responder activity. Communication tools: `alga_publish_status_update`, `alga_complete_task`, `alga_list_tasks`. Do NOT @mention another agent or teammate unless absolutely necessary. Too many unnecessary real agent mentions activate the agent."
	case "responder":
		roleSpecific = "**Your incident role: Responder.** You are the sole owner of technical validation, environment investigation, and recovery commands. The Commander NEVER runs commands or inspects environment/service health; they coordinate, decide, and verify against the evidence you provide. **You receive investigate-kind coordination tasks dispatched by the commander. When dispatched, investigate the assigned goal, publish status milestones via `alga_publish_status_update` as you progress (identified/mitigated/monitoring), and complete the task via `alga_complete_task` with a typed result (finding, hypothesis_confidence, evidence, root_cause_candidate). You may receive multiple parallel tasks. Do NOT call `alga_dispatch_task` (only the commander delegates).** **Focus entirely on recovering the service.** Read the runbook (using `alga_get_knowledge` or the auto-injected shared knowledge) for the exact recovery/mitigation steps and execute them immediately to restore service. **Coordination-tool discipline (NON-NEGOTIABLE — violations break the incident): every call to `alga_post_handoff` ACTIVATES the commander and communicator agents — it interrupts whatever they are doing and triggers ping-pong message loops that prevent the commander from resolving the incident. `alga_post_handoff` is deprecated in favor of `alga_complete_task`; task completion is the normal path for handing work back to the commander. During investigation, identification, mitigation, and verification you are FORBIDDEN from calling `alga_post_handoff` for ANY reason — including posting findings, progress notes, interim summaries, recovery announcements, or status milestones. ALL status communication while you are still working MUST go through `alga_publish_status_update`, which posts to the Status Updates card WITHOUT activating any other agent. The commander monitors the Status Updates card and will act on your `status_level=\"mitigated\"` or `status_level=\"monitoring\"` update. If you need commander attention urgently before your task is done, `alga_post_handoff` with `audience=\"commander\"` is allowed but task completion via `alga_complete_task` is the normal path. Status-level discipline: you may publish `status_level=\"identified\"`, `status_level=\"mitigated\"`, and `status_level=\"monitoring\"` ONLY. You are FORBIDDEN from publishing `status_level=\"resolved\"` (commander-only) or `status_level=\"investigating\"` (system-only). Each milestone is published EXACTLY ONCE per incident — do not re-publish `mitigated` or `monitoring` as a verification update; the commander will publish `resolved` once they verify and resolve. `alga_post_handoff` does NOT accept `status_level`; status milestones must go through `alga_publish_status_update`, not through the handoff metadata.** Workflow (interleave technical steps and milestone status updates): (1) Investigate the environment. As soon as you identify the root cause, you MUST immediately call `alga_publish_status_update` with `status_level=\"identified\"` to post to the **Status Updates card** before executing mitigation. (2) Execute the recovery/mitigation steps. Once the fix is applied and impact is reduced, you MUST immediately call `alga_publish_status_update` with `status_level=\"mitigated\"` to post to the **Status Updates card**. (3) If the fix needs time to confirm full recovery (e.g., waiting on replication to settle, or a multi-hour soak test), call `alga_publish_status_update` with `status_level=\"monitoring\"` to mark the verification phase. **Skip monitoring entirely if the fix is fully verified** — `mitigated` alone is sufficient for handoff. (4) Complete your coordination task via `alga_complete_task` with a typed result (root_cause, resolution, evidence, impact) once you have completed the verification and are ready for the commander to decide resolution. **Handoff/task-result format is strictly structured — do NOT include a `Next Steps`, `Next Steps (for the Incident Commander)`, `Required Actions`, `Required Actions (for incident commander)`, `Commander: ...`, or `Follow-up: ...` section in your coordination messages, findings, or summaries.** Do NOT list commander actions (like \"resolve the incident\" or \"publish a status update\") as next steps or tasks. Your handoff/summary must contain exactly these headers: (a) **Root Cause**: (description of the underlying issue), (b) **Recovery Action**: (actions taken to restore service), (c) **Verification**: (the specific checks you ran and their results — e.g. `pg_isready` output, `pg_stat_replication` rows, error-rate metrics), (d) **Impact**: (duration of impact, user-visible effects, data loss assessment). End the message with the literal line `Ready for commander verification.` Do NOT tell the commander to verify, run, check, log in, restart, or execute anything technical — they don't. If you identified systemic or process issues that need post-incident follow-up (e.g. root-causing a workflow bug), include them briefly under the Impact header; do NOT post additional coordination updates for them. Responder tools: `alga_publish_status_update`, `alga_pause_investigation`, `alga_cancel_investigation`, `alga_complete_task`, `alga_list_tasks` (plus `alga_post_handoff` for urgent commander attention only, as described above). Do not set priority, assign roles, mitigate the incident, or resolve the incident directly. **Reminder: the alert-investigation tool `alga_set_outcome` is used only in alert-scope investigations, do not call it from the incident scope.** **You are FORBIDDEN from calling `alga_resolve_alert` or `alga_reopen_alert` on alerts linked to an active incident — alert closure is part of incident closure and is owned by the incident commander. You are also FORBIDDEN from calling `alga_who_is_on_call` to identify who to hand off to. You do not need to look up who is on call; the handoff in the investigation thread (using `alga_post_handoff` with `audience=\"commander\"` or completing your assigned task) is always directly to the incident commander.** Do NOT @mention another agent or teammate unless absolutely necessary. Too many unnecessary real agent mentions activate the agent."
	default:
		roleSpecific = "**This is an incident-scoped investigation.** Your specific incident role was not identified in the assignment context. Focus on investigation and evidence. For incident state transitions (priority, mitigation, resolution, escalation), coordinate with the incident commander by posting a coordination update with audience=\"commander\" rather than calling commander-only tools directly. When your work is ready for commander verification, post a coordination-thread message with audience=\"commander\". Do not resolve the incident directly. **Reminder: the alert-investigation tool `alga_set_outcome` is used only in alert-scope investigations, do not call it from the incident scope.** Do NOT include a \"Next Steps\" or \"Required Actions\" section in your coordination updates, findings, or summaries. All technical tasks belong to the Responder; the Commander coordinates and verifies but never runs commands. Do NOT @mention another agent or teammate unless absolutely necessary. Too many unnecessary real agent mentions activate the agent."
	}
	out := roleSpecific + "\n\n" + coordinationThreadDiscipline()
	// The commander is the orchestrator and benefits from a phase-aware
	// checklist so it knows what to do next without a separate playbook entity.
	// Other roles don't need it; their work is task-driven by the commander.
	if role == "incident_commander" {
		if checklist := incidentPhaseChecklist(incidentStatus); checklist != "" {
			out += "\n\n" + checklist
		}
	}
	return out
}

// incidentPhaseChecklist returns a short "current phase expectations" block for
// the commander, derived from the incident lifecycle status. It uses
// incident.ExpectedIncidentPhaseActions to avoid duplicating the phase model.
func incidentPhaseChecklist(status string) string {
	actions := incident.ExpectedIncidentPhaseActions(status)
	if len(actions) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Current phase checklist (incident status: %s):**", status)
	for i, a := range actions {
		fmt.Fprintf(&b, "\n%d. %s", i+1, a)
	}
	b.WriteString("\nAdvance through these as the incident progresses. When all items for the current phase are done, the incident is ready for the next lifecycle transition.")
	return b.String()
}

// coordinationThreadDiscipline returns cross-role rules that keep the
// incident coordination thread from devolving into an acknowledgement /
// thank-you loop between agents. Every incident-scoped agent gets this
// appended because any of them can be re-activated by an @mention from a
// teammate.
func coordinationThreadDiscipline() string {
	return "**Coordination thread discipline (avoids ping-pong loops between agents):**\n" +
		"- **An @mention is an activation, not a notification.** Mentioning a teammate wakes their agent up to respond. If your message does not require the mentioned teammate to take an action or make a decision, do NOT mention them. This is the most common cause of ping-pong loops and is non-negotiable. To thank, appreciate, or sign off, write a brief reply without any mention — or write nothing at all.\n" +
		"- Do not @mention a teammate just to thank, acknowledge, agree, sign off, recap for them, or say goodbye. A plain reply without a mention is fine, or no reply at all.\n" +
		"- Do not @mention a teammate to direct them at work they are already doing. If they are already handling it, leave the thread alone.\n" +
		"- Do not respond to a teammate's message that is purely a thank-you, acknowledgement, agreement, greeting, or sign-off. Treat such messages as conversation closers; the thread ends there.\n" +
		"- Do NOT invent teammate names, roles, or UUIDs. Never use template/placeholder names or hypothetical UUIDs like \"123e4567-e89b-12d3-a456-426614174000\" in mentions. Only mention a teammate if you have retrieved their exact Agent ID or User ID from the active roles list (via the `alga_get_incident_context` tool or previous in-thread messages). If a teammate is not in the active roles list, do NOT reference or mention them.\n" +
		"- Once the incident has reached `resolved` or `closed`, do not post further coordination messages unless a human operator asks you to. Do not post goodbye, recap, or \"nice working with you\" messages after resolution.\n" +
		"- Prefer tool calls over chat: use `alga_publish_status_update` or `alga_post_handoff` instead of a free-text @mention whenever the dedicated tool exists for the action.\n" +
		"- If you have already posted your contribution and are waiting on a teammate, stop and wait. Do not post status checks or follow-ups that re-@mention them."
}

// toolThreadChatID returns the owner-scoped chat_id that alga_* tools expect
// as their `investigation_id` parameter: `alert_<n>` for alert investigations
// and `incident_coord_<n>` for incident investigations. It mirrors the
// owner-chat-id resolution in the backend's investigation forwarder. Returns
// an empty string when no number is available yet.
func toolThreadChatID(in DispatchInput) string {
	if in.IncidentScope {
		if in.IncidentNumber > 0 {
			return fmt.Sprintf("incident_coord_%d", in.IncidentNumber)
		}
		return ""
	}
	if in.PrimaryAlertNumber > 0 {
		return fmt.Sprintf("alert_%d", in.PrimaryAlertNumber)
	}
	if len(in.Alerts) > 0 && in.Alerts[0].AlertNumber > 0 {
		return fmt.Sprintf("alert_%d", in.Alerts[0].AlertNumber)
	}
	return ""
}

func serviceDownGuidance(alerts []rabbitmq.CorrelatedAlert, incidentScope bool) string {
	for _, a := range alerts {
		name := strings.ToLower(strings.TrimSpace(a.Labels["alertname"]))
		if name == "" {
			continue
		}
		if strings.Contains(name, "down") {
			var b strings.Builder
			b.WriteString("## Service-Down Investigation Response\n")
			b.WriteString("Do not act on this alert blindly, and do not mitigate before verifying it or before applying the runbook's promotion directive. Follow this order:\n")
			b.WriteString("1. **Verify the alert first.** Confirm the target is genuinely down (a real service-level check, not just a metric flap), the alert is still firing, and it is not stale or already recovered. Re-check the linked alert numbers with `alga_list_alerts`; if every linked alert is already `resolved`, finalize the investigation with `alga_set_outcome` and stop.\n")
			b.WriteString("2. **Read the runbook** (auto-injected under SHARED KNOWLEDGE, or via `alga_get_knowledge`) for its promotion criteria and recovery steps. The runbook is the source of truth for service-specific recovery steps and promotion criteria; do not invent or hardcode service-specific recovery commands.\n")
			if incidentScope {
				b.WriteString("3. **Focus on recovering the service immediately.** Read the runbook for the exact recovery/mitigation steps and execute them to restore service; verify recovery with service-specific health checks. Escalate to the incident commander if blocked.\n")
				b.WriteString("4. Record RCA after recovery or handoff.\n")
			} else {
				b.WriteString("3. **Apply the promotion directive before mitigating.** Promotion is warranted when EITHER (a) the runbook mandates it (e.g. \"any node down → promote immediately\") OR (b) there is real, current **user-facing** impact AND the alert is still firing. In both cases, call `alga_promote_to_incident` now — do not mitigate, validate recovery, or record outcomes before promoting. A runbook is not required: alerts without a matching runbook are promoted based on confirmed user-facing impact. **After calling `alga_promote_to_incident`, stop immediately — do not run any further SSH commands, log checks, or tool calls. The incident response team owns all follow-up from this point.**\n")
				b.WriteString("4. **Recovery flow — only if you did NOT promote in step 3:** attempt the runbook's safe mitigation steps and verify recovery with service-specific health checks. If user-facing impact emerges during recovery, promote via `alga_promote_to_incident` and stop. Once the alert recovers without promoting, finalize the investigation via `alga_resolve_alert` and `alga_set_outcome`.\n")
				b.WriteString("5. Record RCA after recovery or handoff.\n")
			}
			return b.String()
		}
	}
	return ""
}

func applyStoredTriageEnrichment(in *DispatchInput, enrichment map[string]any) {
	if len(enrichment) == 0 {
		return
	}
	in.ServiceOwner = stringFromAny(enrichment["service_owner"])
	in.RunbookURL = stringFromAny(enrichment["runbook_url"])
	in.SuggestedActions = stringSliceFromAny(enrichment["suggested_actions"])
	in.SimilarInvestigationIDs = stringSliceFromAny(enrichment["similar_investigation_ids"])
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func stringSliceFromAny(v any) []string {
	switch vals := v.(type) {
	case []string:
		return vals
	case []any:
		out := make([]string, 0, len(vals))
		for _, val := range vals {
			if s, ok := val.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func alertDetailLimit(severity string, total int) int {
	if total <= 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high", "page":
		return total
	case "warning", "warn":
		return min(total, 5)
	case "info", "informational", "low":
		return min(total, 1)
	default:
		return min(total, 3)
	}
}

func investigationDepthHint(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high", "page":
		return "full-depth root cause analysis; validate impact and escalate quickly if blocked"
	case "warning", "warn":
		return "focused investigation; confirm impact, likely cause, and safe remediation"
	case "info", "informational", "low":
		return "lightweight validation; check for noise, suppression candidates, and recurring patterns"
	default:
		return "balanced investigation; identify cause, impact, and next action"
	}
}

func diffLabels(all, common map[string]string) map[string]string {
	if len(common) == 0 {
		return all
	}
	diff := map[string]string{}
	for k, v := range all {
		if common[k] != v {
			diff[k] = v
		}
	}
	if len(diff) == 0 {
		return nil
	}
	return diff
}
