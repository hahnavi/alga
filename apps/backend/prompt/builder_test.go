package prompt

import (
	"strings"
	"testing"

	"alga/rabbitmq"
	"alga/store"
)

func TestBuildDispatchPromptUsesOwnerResolution(t *testing.T) {
	p := BuildDispatchPrompt(DispatchInput{InvestigationID: "AINV-1"})
	if strings.Contains(p, "alga_complete_investigation") {
		t.Fatalf("prompt must not mention alga_complete_investigation:\n%s", p)
	}
	if strings.Contains(p, "Resolve first, then document") {
		t.Fatalf("prompt must not encourage resolving before verification:\n%s", p)
	}
	if !strings.Contains(p, "Verify recovery before resolving") {
		t.Fatalf("prompt should require verification before resolving:\n%s", p)
	}
	if !strings.Contains(p, "alga_resolve_alert") {
		t.Fatalf("prompt should tell agent to resolve alerts:\n%s", p)
	}
}

func TestBuildDispatchPromptUsesCommanderVerificationForIncidentScope(t *testing.T) {
	p := BuildDispatchPrompt(DispatchInput{InvestigationID: "AINV-1", IncidentScope: true, IncidentID: "1"})
	if strings.Contains(p, "alga_resolve_incident") {
		t.Fatalf("incident investigator prompt must not mention alga_resolve_incident:\n%s", p)
	}
	for _, want := range []string{"commander", "ready for commander verification", "coordination"} {
		if !strings.Contains(strings.ToLower(p), strings.ToLower(want)) {
			t.Fatalf("incident prompt missing %q:\n%s", want, p)
		}
	}
	for _, forbidden := range []string{"alga_mitigate_incident", "alga_resolve_incident", "alga_set_incident_priority"} {
		if strings.Contains(p, forbidden) {
			t.Fatalf("default incident prompt advertised commander-only tool %q:\n%s", forbidden, p)
		}
	}
}

func TestBuildDispatchPromptIncidentResponderToolsAreRoleAware(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-1",
		IncidentScope:   true,
		IncidentRole:    "responder",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	for _, want := range []string{
		"Your incident role: Responder",
		"alga_publish_status_update",
		"Status Updates card",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("responder prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"alga_set_incident_priority", "alga_resolve_incident", "alga_mitigate_incident", "alga_add_finding", "alga_report_to_communicator"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("responder prompt advertised forbidden tool %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptResponderHandoffInstructions(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-r",
		IncidentScope:   true,
		IncidentRole:    "responder",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	// alga_add_finding is recommended but optional when the summary already captures the milestone.
	for _, want := range []string{
		"Status Updates card",
		"Ready for commander verification.",
		"mitigated",
		"monitoring",
		"alga_publish_status_update",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("responder prompt missing %q:\n%s", want, prompt)
		}
	}
	// The old mandatory gate that blocked the commander when alga_add_finding was absent must be gone.
	for _, forbidden := range []string{
		"Posting the commander-handoff without a prior `alga_add_finding`",
		"The handoff to the Communicator is mandatory before the commander-handoff",
		"the commander will be blocked from resolving",
		"alga_add_finding",
		"alga_report_to_communicator",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("responder prompt must not contain hard gate or forbidden tool %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptResponderForbidsCommanderStepsInHandoff(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-1",
		IncidentScope:   true,
		IncidentRole:    "responder",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	for _, want := range []string{
		"Do NOT tell the commander to verify, run, check, log in, restart",
		"Ready for commander verification.",
		"the alert-investigation tool `alga_set_outcome` is used only in alert-scope investigations, do not call it from the incident scope",
		"do NOT include a `Next Steps`, `Next Steps (for the Incident Commander)`, `Required Actions`, `Required Actions (for incident commander)`",
		"Do NOT list commander actions (like \"resolve the incident\" or \"publish a status update\") as next steps or tasks",
		"exactly these headers",
		"Root Cause",
		"Recovery Action",
		"Verification",
		"Impact",
		"If you identified systemic or process issues that need post-incident follow-up",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("responder prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildDispatchPromptResponderForbidsCoordinationUpdateDuringInvestigation(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-1",
		IncidentScope:   true,
		IncidentRole:    "responder",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	for _, want := range []string{
		"Coordination-tool discipline (NON-NEGOTIABLE",
		"ACTIVATES the commander and communicator agents",
		"FORBIDDEN from calling `alga_post_handoff` for ANY reason",
		"ALL status communication while you are still working MUST go through `alga_publish_status_update`",
		"`alga_post_handoff` is deprecated in favor of `alga_complete_task`",
		"FORBIDDEN from publishing `status_level=\"resolved\"` (commander-only) or `status_level=\"investigating\"` (system-only)",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("responder prompt missing coordination-discipline rule %q:\n%s", want, prompt)
		}
	}

	// The Available Tools section must NOT advertise alga_post_handoff
	// to the responder (it gets re-introduced under strict rules lower down, but
	// the top-of-prompt listing must not normalize it).
	toolsIdx := strings.Index(prompt, "## Available Tools")
	if toolsIdx == -1 {
		t.Fatalf("missing ## Available Tools section in responder prompt")
	}
	instructionsIdx := strings.Index(prompt, "## Incident Instructions")
	if instructionsIdx == -1 {
		t.Fatalf("missing ## Incident Instructions section in responder prompt")
	}
	availableSection := prompt[toolsIdx:instructionsIdx]
	if strings.Contains(availableSection, "alga_post_handoff") {
		t.Fatalf("responder Available Tools section must not list alga_post_handoff:\n%s", availableSection)
	}
	if !strings.Contains(availableSection, "alga_publish_status_update") {
		t.Fatalf("responder Available Tools section must list alga_publish_status_update:\n%s", availableSection)
	}
}

func TestBuildDispatchPromptCommanderAdvertisesCoordinationUpdate(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-c",
		IncidentScope:   true,
		IncidentRole:    "incident_commander",
		Severity:        "critical",
	})

	toolsIdx := strings.Index(prompt, "## Available Tools")
	instructionsIdx := strings.Index(prompt, "## Incident Instructions")
	if toolsIdx == -1 || instructionsIdx == -1 {
		t.Fatalf("missing expected prompt sections")
	}
	availableSection := prompt[toolsIdx:instructionsIdx]
	if !strings.Contains(availableSection, "alga_post_handoff") {
		t.Fatalf("commander Available Tools section must list alga_post_handoff:\n%s", availableSection)
	}
}

func TestBuildDispatchPromptCommanderCannotRunTechnicalCommands(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-cmd",
		IncidentScope:   true,
		IncidentRole:    "incident_commander",
		Severity:        "critical",
	})

	for _, want := range []string{
		"Commander verification is a paper review",
		"do not run commands yourself",
		"`Ready for commander verification.`",
		"Do NOT @mention the Responder or any other teammate in appreciation",
		"An @mention activates them and forces a ping-pong reply",
		"\"Great work, @Responder\" message is forbidden",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("commander prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"Verify responder recovery and ensure"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("commander prompt must not use ambiguous 'verify' wording (%q):\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptCoordinationDisciplineForbidsMentionForAck(t *testing.T) {
	for _, role := range []string{"responder", "incident_commander", "communications_lead", ""} {
		prompt := BuildDispatchPrompt(DispatchInput{
			InvestigationID: "iinv-disc",
			IncidentScope:   true,
			IncidentRole:    role,
		})
		for _, want := range []string{
			"An @mention is an activation, not a notification",
			"Do not @mention a teammate just to thank, acknowledge, agree, sign off, recap for them, or say goodbye",
		} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("role %q prompt missing %q:\n%s", role, want, prompt)
			}
		}
	}
}

func TestBuildDispatchPromptIncidentCommunicatorIsTaskDriven(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-comms",
		IncidentScope:   true,
		IncidentRole:    "communications_lead",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	// The communications lead is now task-driven: it wakes on dispatched
	// communicate-kind tasks and completes them, but does not act proactively.
	for _, want := range []string{
		"Your incident role: Communications Lead",
		"communicate-kind coordination tasks dispatched by the commander",
		"coordination_task_dispatched event",
		"complete the task via `alga_complete_task`",
		"NOT expected to take proactive action on incident progression",
		"Do NOT @mention another agent or teammate unless absolutely necessary",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("task-driven communicator prompt missing %q:\n%s", want, prompt)
		}
	}
	// The old active-communicator instructions must be gone.
	for _, forbidden := range []string{
		"sole author of public status updates",
		"alga_request_status_update",
		"incident_comms_task",
		"When you receive an `incident_comms_task` from the commander",
		"remind the commander to request a status update via `alga_request_status_update`",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("task-driven communicator prompt must not contain active-communicator instruction %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptPromotedAlertHandoffNamesIncidentNumber(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID:                 "AINV-49",
		IncidentScope:                   true,
		IncidentID:                      "25",
		IncidentNumber:                  25,
		PromotedAlertHandoff:            true,
		PromotedIncidentInvestigationID: "IINV-25",
		Alerts: []rabbitmq.CorrelatedAlert{{
			Fingerprint: "fp-test345",
			AlertNumber: 49,
			Labels:      map[string]string{"alertname": "Test345"},
		}},
	})

	for _, want := range []string{
		"This alert investigation has been promoted to an incident",
		"Incident **#25**",
		"investigation is now complete",
		"handled by a separate incident investigation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("promoted handoff prompt missing %q:\n%s", want, prompt)
		}
	}

	// The alert-investigation agent must not be told to keep working in the
	// incident scope, and incident-only tools must not be advertised.
	for _, forbidden := range []string{
		"IINV-25",
		"Continue the active investigation",
		"use the incident coordination thread",
		"alga_resolve_incident",
		"alga_mitigate_incident",
		"alga_post_handoff",
		"Incident Commander",
		"ask the user whether to continue",
		"whether to continue",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("promoted handoff prompt must not contain %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptPromotedAlertHandoffFallsBackWithoutIncidentNumber(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID:                 "AINV-50",
		IncidentScope:                   true,
		PromotedAlertHandoff:            true,
		PromotedIncidentInvestigationID: "IINV-50",
		Alerts: []rabbitmq.CorrelatedAlert{{
			Fingerprint: "fp-test456",
			AlertNumber: 50,
			Labels:      map[string]string{"alertname": "Test456"},
		}},
	})

	for _, want := range []string{
		"This alert investigation has been promoted to an incident",
		"investigation is now complete",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("promoted handoff fallback prompt missing %q:\n%s", want, prompt)
		}
	}

	// Without an incident number we must not leak the incident investigation UUID.
	for _, forbidden := range []string{
		"IINV-50",
		"Continue the active investigation",
		"alga_resolve_incident",
		"whether to continue",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("promoted handoff fallback prompt must not contain %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptCommanderPublishesResolvedStatusUpdateDirectly(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-c",
		IncidentScope:   true,
		IncidentRole:    "incident_commander",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	for _, want := range []string{
		"Commander tools: `alga_set_incident_priority`",
		"publish a public-consumable status update with `status_level=\"resolved\"` directly via `alga_publish_status_update`",
		"You are the sole role responsible for publishing the \"resolved\" status update; do not ask the responder to do it",
		// Commander evidence check: summary and thread together are sufficient — no mandatory alga_add_finding.
		"the investigation summary, coordination messages, and responder updates together constitute the evidence",
		"you do NOT require any formal findings tool calls as findings/outcomes are gathered directly from the coordination thread",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("commander prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"Defer public status updates to the assigned Communications Lead",
		"do NOT call `alga_publish_status_update` yourself for `investigating`, `identified`, or `monitoring` milestones",
		"alga_report_to_communicator",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("commander prompt must not contain %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptCommunicatorHasNoAutoPublishInstruction(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-comms-2",
		IncidentScope:   true,
		IncidentRole:    "communications_lead",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	// The old "remind the commander to request a status update" instruction
	// implied the communicator actively polls for missing requests. The new
	// passive role does not.
	for _, forbidden := range []string{
		"do NOT receive an `incident_comms_task`",
		"remind the commander to request a status update via `alga_request_status_update`",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("passive communicator prompt must not contain active polling instruction %q:\n%s", forbidden, prompt)
		}
	}
	for _, forbidden := range []string{"alga_report_to_communicator"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("communicator prompt must not reference removed tool %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptIncidentCommunicatorOmitsCommsTaskReferences(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-comms",
		IncidentScope:   true,
		IncidentRole:    "communications_lead",
		Alerts: []rabbitmq.CorrelatedAlert{{
			Fingerprint: "fp-1",
			Labels:      map[string]string{"alertname": "PostgreSQLDown"},
		}},
	})

	// The old prompt referenced `incident_comms_task` and
	// `alga_publish_status_update` as active behaviors. The passive role keeps
	// `alga_publish_status_update` available for human-directed use but drops
	// the auto-publish trigger.
	for _, forbidden := range []string{"incident_comms_task"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("passive communicator prompt must not contain %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptIncidentScopeWithoutPromotedHandoffDoesNotClaimPromotion(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "IINV-25",
		IncidentScope:   true,
		IncidentID:      "25",
		IncidentRole:    "responder",
	})

	if strings.Contains(prompt, "This alert investigation has been promoted") {
		t.Fatalf("regular incident prompt should not claim alert promotion:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Your incident role: Responder") {
		t.Fatalf("regular incident prompt missing role instructions:\n%s", prompt)
	}
}

func TestBuildDispatchPromptIncidentScopeServiceDownOmitsAlertPromotionGuidance(t *testing.T) {
	// Incident-scoped *Down investigations must not carry alert-only promotion
	// guidance: promotion to an incident is meaningless there and alga_set_outcome
	// and alga_add_finding are not offered in incident scope. Only the
	// mitigation-first recovery steps should appear.
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "IINV-26",
		IncidentScope:   true,
		IncidentID:      "26",
		IncidentRole:    "responder",
		Severity:        "critical",
		Alerts: []rabbitmq.CorrelatedAlert{{
			AlertNumber: 35,
			Status:      "firing",
			Fingerprint: "fp-35",
			Labels: map[string]string{
				"alertname": "PostgreSQLDown",
				"instance":  "pg2",
				"severity":  "critical",
			},
		}},
	})

	if !strings.Contains(prompt, "Verify the alert first") {
		t.Fatalf("incident service-down prompt must still carry verify-first recovery steps:\n%s", prompt)
	}
	for _, forbidden := range []string{
		"Verify before acting, then follow the runbook for promotion",
		"Apply the promotion directive before mitigating",
		"alga_promote_to_incident",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("incident-scoped service-down prompt must not carry alert-promotion guidance %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptIncidentCommanderToolsAreRoleAware(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-2",
		IncidentScope:   true,
		IncidentRole:    "incident_commander",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	for _, want := range []string{
		"Your incident role: Incident Commander",
		"alga_set_incident_priority",
		"alga_trigger_escalation",
		"alga_resolve_incident",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("commander prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildDispatchPromptIncidentDefaultDoesNotAdvertiseCommanderTools(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-3",
		IncidentScope:   true,
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	for _, want := range []string{
		"incident-scoped investigation",
		"commander",
		"coordination",
		"audience=\"commander\"",
		"Do NOT include a \"Next Steps\" or \"Required Actions\" section in your coordination updates, findings, or summaries",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("default incident prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"alga_mitigate_incident", "alga_resolve_incident", "alga_set_incident_priority"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("default incident prompt advertised commander-only tool %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptAdvertisesKnowledgeReadTool(t *testing.T) {
	p := BuildDispatchPrompt(DispatchInput{InvestigationID: "AINV-1"})

	if !strings.Contains(p, "alga_get_knowledge") {
		t.Fatalf("prompt must advertise alga_get_knowledge:\n%s", p)
	}
	if !strings.Contains(p, "alga_search_knowledge") {
		t.Fatalf("prompt must advertise alga_search_knowledge:\n%s", p)
	}
	for _, want := range []string{"Read runbooks before debugging", "follow its steps"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing knowledge instruction %q:\n%s", want, p)
		}
	}
}

func TestBuildDispatchPromptServiceDownGuidesReadingRunbook(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "ainv-1",
		Severity:        "critical",
		Alerts: []rabbitmq.CorrelatedAlert{{
			AlertNumber: 44,
			Status:      "firing",
			Fingerprint: "fp-44",
			Labels: map[string]string{
				"alertname": "PostgreSQLDown",
				"instance":  "pg2",
				"severity":  "critical",
			},
		}},
	})

	if !strings.Contains(prompt, "call `alga_get_knowledge`") {
		t.Fatalf("service-down prompt must instruct reading the full runbook via alga_get_knowledge:\n%s", prompt)
	}
	if !strings.Contains(prompt, "The runbook is the source of truth") {
		t.Fatalf("service-down prompt must defer service-specific recovery to the runbook, not hardcode it:\n%s", prompt)
	}
}

func TestBuildDispatchPromptOmitsEmptyOptionalContext(t *testing.T) {
	p := BuildDispatchPrompt(DispatchInput{InvestigationID: "AINV-1"})

	for _, emptyField := range []string{"**Severity:** \n", "**Impact:** \n", "**Priority:** \n"} {
		if strings.Contains(p, emptyField) {
			t.Fatalf("prompt should omit empty optional field %q:\n%s", emptyField, p)
		}
	}
}

func TestFromInvestigateMessageCarriesTriageEnrichment(t *testing.T) {
	in := FromInvestigateMessage(rabbitmq.InvestigateMessage{
		InvestigationID: "AINV-1",
		Severity:        "critical",
		TriageEnrichment: rabbitmq.TriageEnrichment{
			ServiceOwner:            "payments-team",
			RunbookURL:              "https://runbooks/payments",
			SuggestedActions:        []string{"check payment gateway errors"},
			SimilarInvestigationIDs: []string{"AINV-OLD"},
		},
	})

	p := BuildDispatchPrompt(in)
	for _, want := range []string{"payments-team", "https://runbooks/payments", "check payment gateway errors", "AINV-OLD"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing triage enrichment %q:\n%s", want, p)
		}
	}
}

func TestFromAlertInvestigationRecordCarriesTriageEnrichment(t *testing.T) {
	in := FromAlertInvestigationRecord(&store.AlertInvestigationRecord{
		AlertInvestigationID: "AINV-1",
		TriageEnrichment: map[string]any{
			"service_owner":             "platform-team",
			"runbook_url":               "https://runbooks/platform",
			"suggested_actions":         []any{"check recent deploys"},
			"similar_investigation_ids": []any{"AINV-7"},
		},
	})

	p := BuildDispatchPrompt(in)
	for _, want := range []string{"platform-team", "https://runbooks/platform", "check recent deploys", "AINV-7"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing stored triage enrichment %q:\n%s", want, p)
		}
	}
}

func TestBuildDispatchPromptCriticalServiceDownMitigationFirst(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "ainv-1",
		Severity:        "critical",
		Alerts: []rabbitmq.CorrelatedAlert{{
			AlertNumber: 35,
			Status:      "firing",
			Fingerprint: "fp-35",
			Labels: map[string]string{
				"alertname": "PostgreSQLDown",
				"instance":  "pg2",
				"severity":  "critical",
			},
			Annotations: map[string]string{
				"summary": "PostgreSQL instance pg2 is not responding.",
			},
		}},
	})

	for _, want := range []string{
		"Do not act on this alert blindly",
		"Verify the alert first",
		"Apply the promotion directive before mitigating",
		"alga_list_alerts",
		"alga_set_outcome",
		"alga_get_knowledge",
		"The runbook is the source of truth",
		"Verify before acting, then follow the runbook for promotion",
		"A runbook is not required for promotion",
		"user-facing",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"A runbook rule that says \"any node down → promote immediately\" does NOT override",
		"A runbook rule does NOT override the promotion gate",
		"Patroni",
		"pg_isready",
		"pg_wal",
		"kubectl",
		"docker",
		"redis-cli",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("service-down prompt must not carry runbook-override / hardcoded guidance (%q):\n%s", forbidden, prompt)
		}
	}
}

// TestBuildDispatchPromptAlertScopeProactiveUserFacingPromotionDefault guards
// against the regression where promotion was made contingent on an explicit
// runbook mandate. Many alerts have no matching runbook, so the default must
// allow promotion on real, current user-facing impact as an independent
// trigger — a runbook is authoritative when it speaks, but NOT required.
func TestBuildDispatchPromptAlertScopeProactiveUserFacingPromotionDefault(t *testing.T) {
	p := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "AINV-99",
		Severity:        "critical",
		Alerts: []rabbitmq.CorrelatedAlert{{
			AlertNumber: 99,
			Status:      "firing",
			Fingerprint: "fp-99",
			Labels:      map[string]string{"alertname": "PostgreSQLDown", "instance": "pg3", "severity": "critical"},
		}},
	})

	for _, want := range []string{
		"A runbook is not required for promotion",
		"user-facing",
		"alerts without a matching runbook are promoted based on confirmed user-facing impact",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("alert-scope prompt must carry proactive user-facing-impact default %q:\n%s", want, p)
		}
	}
	for _, forbidden := range []string{
		"do NOT promote. Resolve the alert in this investigation",
		"Promotion requires an explicit runbook mandate",
	} {
		if strings.Contains(p, forbidden) {
			t.Fatalf("alert-scope prompt must not default-deny promotion when runbook is silent %q:\n%s", forbidden, p)
		}
	}
}

func TestBuildDispatchPromptServiceDownAccumulatesAcrossCorrelatedAlerts(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "ainv-1",
		Severity:        "critical",
		Alerts: []rabbitmq.CorrelatedAlert{
			{
				AlertNumber: 41,
				Status:      "firing",
				Fingerprint: "fp-41",
				Labels: map[string]string{
					"alertname": "RedisDown",
					"instance":  "redis1",
					"severity":  "critical",
				},
				Annotations: map[string]string{
					"summary": "Redis instance redis1 is not responding.",
				},
			},
			{
				AlertNumber: 42,
				Status:      "firing",
				Fingerprint: "fp-42",
				Labels: map[string]string{
					"alertname": "PostgreSQLDown",
					"instance":  "pg2",
					"severity":  "critical",
				},
				Annotations: map[string]string{
					"summary": "PostgreSQL instance pg2 is not responding.",
				},
			},
		},
	})

	for _, want := range []string{
		"Do not act on this alert blindly",
		"alga_set_outcome",
		"alga_list_alerts",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"Patroni",
		"pg_isready",
		"pg_wal",
		"kubectl",
		"docker",
		"redis-cli",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("service-down prompt must not hardcode service-specific recovery commands (%q):\n%s", forbidden, prompt)
		}
	}
}

func TestBuildDispatchPromptNoMitigationForNonServiceDownAlerts(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "ainv-1",
		Severity:        "warning",
		Alerts: []rabbitmq.CorrelatedAlert{{
			AlertNumber: 50,
			Status:      "firing",
			Fingerprint: "fp-50",
			Labels: map[string]string{
				"alertname": "HighCPU",
				"instance":  "node1",
				"severity":  "warning",
			},
			Annotations: map[string]string{
				"summary": "CPU usage on node1 above 90%.",
			},
		}},
	})

	if strings.Contains(prompt, "Mitigation-First Response") {
		t.Fatalf("non-service-down prompt must not include mitigation-first guidance:\n%s", prompt)
	}
}

func TestBuildDispatchPromptIncidentScopeAlertClosureVisibility(t *testing.T) {
	// Commander prompt: must advertise alga_resolve_alert in available tools so the commander can close the linked alert as part of incident closure.
	pCommander := BuildDispatchPrompt(DispatchInput{InvestigationID: "AINV-1", IncidentScope: true, IncidentRole: "incident_commander"})
	toolsIdxCommander := strings.Index(pCommander, "## Available Tools")
	if toolsIdxCommander == -1 {
		t.Fatalf("missing ## Available Tools section in commander prompt")
	}
	toolsSectionCommander := pCommander[toolsIdxCommander:]
	if !strings.Contains(toolsSectionCommander, "alga_resolve_alert") {
		t.Fatalf("incident commander prompt must advertise alga_resolve_alert in available tools for alert closure:\n%s", pCommander)
	}

	// Responder prompt: must hide alga_resolve_alert from the incident investigation thread and warn against resolve_alert/who_is_on_call.
	pResponder := BuildDispatchPrompt(DispatchInput{InvestigationID: "AINV-1", IncidentScope: true, IncidentRole: "responder"})
	toolsIdxResponder := strings.Index(pResponder, "## Available Tools")
	if toolsIdxResponder == -1 {
		t.Fatalf("missing ## Available Tools section in responder prompt")
	}
	toolsSectionResponder := pResponder[toolsIdxResponder:]
	if idx := strings.Index(toolsSectionResponder, "## Incident Instructions"); idx != -1 {
		toolsSectionResponder = toolsSectionResponder[:idx]
	}
	if strings.Contains(toolsSectionResponder, "alga_resolve_alert") {
		t.Fatalf("incident responder prompt should not advertise alga_resolve_alert in available tools:\n%s", pResponder)
	}
	if !strings.Contains(pResponder, "FORBIDDEN from calling `alga_resolve_alert`") {
		t.Fatalf("incident responder prompt should warn agent that they are forbidden from resolving the alert")
	}
	if !strings.Contains(pResponder, "FORBIDDEN from calling `alga_who_is_on_call`") {
		t.Fatalf("incident responder prompt should warn agent that they are forbidden from calling who is on call")
	}
}

func TestBuildDispatchPromptIncidentCommanderOwnsAlertClosure(t *testing.T) {
	prompt := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-cmd-close",
		IncidentScope:   true,
		IncidentRole:    "incident_commander",
		Severity:        "critical",
		Alerts:          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", Labels: map[string]string{"alertname": "PostgreSQLDown"}}},
	})

	for _, want := range []string{
		// Step ordering: evidence check, alert verification+closure, then status update, then resolve incident.
		"(1) confirm the Responder's evidence is complete and consistent",
		"(2) verify the linked alert's status with `alga_list_alerts` and, if it is still firing, close it with `alga_resolve_alert`",
		"(3) publish a public-consumable status update with `status_level=\"resolved\"`",
		"(5) call `alga_resolve_incident`",
		// Commander owns alert closure — explicitly forbidden from delegating it back.
		"You also own alert closure as part of incident closure — do not ask the Responder to resolve the alert",
		// Alert state checks and closure are framed as part of closure, not technical actions.
		"Alert state checks (`alga_list_alerts`) and closure (`alga_resolve_alert`) are part of incident closure, not technical actions",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("commander prompt missing %q:\n%s", want, prompt)
		}
	}

	// The old "alert tools forbidden" wording must be gone — the commander now uses alert tools for closure.
	for _, forbidden := range []string{
		"or accessing alert tools (such as alert resolution/reopening)",
		"All technical work, commands, validation, and alert resolution must be handled by the Responder",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("commander prompt must not contain stale alert-forbidden wording %q:\n%s", forbidden, prompt)
		}
	}
}

// TestBuildDispatchPromptShowsToolThreadChatID guards against the agent
// confusing the random-UUID Investigation ID with the owner-scoped chat_id
// that alga_* tools require as their investigation_id parameter. The prompt
// must surface the canonical alert_N / incident_coord_N tool thread.
func TestBuildDispatchPromptShowsToolThreadChatID(t *testing.T) {
	p := BuildDispatchPrompt(DispatchInput{
		InvestigationID:    "9a1089ba-4918-4426-be59-8e858ab063ff",
		PrimaryAlertNumber: 85,
	})
	for _, want := range []string{
		"**Tool thread:** alert_85",
		"NOT the Investigation ID",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("alert-scope prompt missing %q:\n%s", want, p)
		}
	}
}

func TestBuildDispatchPromptShowsToolThreadChatIDForIncidentScope(t *testing.T) {
	p := BuildDispatchPrompt(DispatchInput{
		InvestigationID: "iinv-7",
		IncidentScope:   true,
		IncidentNumber:  7,
	})
	if !strings.Contains(p, "**Tool thread:** incident_coord_7") {
		t.Fatalf("incident-scope prompt missing tool thread line:\n%s", p)
	}
}
