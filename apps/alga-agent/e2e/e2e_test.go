package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestAlgaAgentE2E drives the full stack: mint an agent token, run the real
// agent in-process against a real LLM, create an alert, and assert the agent
// participates in the alert's investigation thread. Subtests run sequentially
// and share one alert and agent token — the mention flow depends on the
// scheduler having assigned the agent during the dispatch flow. Tool-effect
// scenarios (outcome, resolve, reopen, promote) are gated behind
// ALGA_AGENT_E2E_TOOLS=1 because they depend on model tool-use quality.
func TestAlgaAgentE2E(t *testing.T) {
	serverURL := requireE2E(t)
	cfg := loadAgentConfig(t, serverURL)
	runID := fmt.Sprintf("%d", time.Now().UnixNano())

	bc, err := newBackendClient(serverURL)
	if err != nil {
		t.Fatalf("backend client: %v", err)
	}
	if err := bc.setupOrLogin("e2e-admin@alga.local", e2eAdminPassword(), "E2E Admin"); err != nil {
		t.Fatalf("setup/login: %v", err)
	}

	tokenID, token, err := bc.mintAgentToken("e2e-agent-" + runID)
	if err != nil {
		t.Fatalf("mint agent token: %v", err)
	}
	t.Logf("agent token id: %s", tokenID)

	startAgent(t, cfg, token)

	waitFor(t, 30*time.Second, "agent to come online (live SSE)", func() (bool, error) {
		return bc.agentOnline(tokenID)
	})

	alertNumber, _, err := bc.createAlert(
		"e2e-agent-"+runID,
		"critical",
		"Synthetic alert created by the alga-agent E2E test. Briefly acknowledge it in the thread. Do not resolve or close it.",
	)
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}
	t.Logf("alert #%d", alertNumber)

	agentMessages := func(n int64) ([]threadMessage, error) {
		msgs, err := bc.getThread(n)
		if err != nil {
			return nil, err
		}
		var out []threadMessage
		for _, m := range msgs {
			if m.Source == "agent" && strings.TrimSpace(m.Message) != "" {
				out = append(out, m)
			}
		}
		return out, nil
	}

	// waitForDispatchReply waits for the scheduler to dispatch the alert to
	// the agent and for the agent to post its first reply in the thread.
	waitForDispatchReply := func(t *testing.T, n int64) {
		t.Helper()
		waitFor(t, 120*time.Second, fmt.Sprintf("scheduler dispatch and an agent reply in alert #%d thread", n), func() (bool, error) {
			msgs, err := agentMessages(n)
			if err != nil {
				return false, err
			}
			return len(msgs) > 0, nil
		})
	}

	dispatchOK := t.Run("dispatch_reply", func(t *testing.T) {
		waitForDispatchReply(t, alertNumber)
	})

	// requireTools gates the tool-effect scenarios: they depend on model
	// tool-use quality (may flake) and on the agent having been assigned to
	// the shared alert's investigation by the dispatch flow.
	requireTools := func(t *testing.T) {
		t.Helper()
		if os.Getenv("ALGA_AGENT_E2E_TOOLS") != "1" {
			t.Skip("skipping: set ALGA_AGENT_E2E_TOOLS=1 to test tool effects (model-dependent, may flake)")
		}
		if !dispatchOK {
			t.Skip("skipping: dispatch_reply failed, dispatch pipeline is not working")
		}
	}

	t.Run("mention_reply", func(t *testing.T) {
		if !dispatchOK {
			t.Skip("skipping: dispatch_reply failed, agent was never assigned to the investigation")
		}
		canary := "E2E-CANARY-" + runID
		prompt := fmt.Sprintf("Please reply with exactly the marker %s and nothing else. Do not call any tools.", canary)
		if err := bc.postThreadMessage(alertNumber, prompt, []string{"agent:" + tokenID}); err != nil {
			t.Fatalf("post mention: %v", err)
		}
		// Match on the canary itself rather than message count: the dispatch
		// turn may still be posting messages after dispatch_reply passes.
		waitFor(t, 90*time.Second, "an agent reply containing the canary "+canary, func() (bool, error) {
			msgs, err := agentMessages(alertNumber)
			if err != nil {
				return false, err
			}
			for _, m := range msgs {
				if strings.Contains(m.Message, canary) {
					return true, nil
				}
			}
			return false, nil
		})
	})

	t.Run("outcome_tool", func(t *testing.T) {
		requireTools(t)
		rootCanary := "E2E-ROOTCAUSE-" + runID
		resCanary := "E2E-RESOLUTION-" + runID
		prompt := fmt.Sprintf("Record the investigation outcome now using your set outcome tool: set the root cause to exactly %q and the resolution to exactly %q. Do not resolve or close the alert.", rootCanary, resCanary)
		if err := bc.postThreadMessage(alertNumber, prompt, []string{"agent:" + tokenID}); err != nil {
			t.Fatalf("post mention: %v", err)
		}
		waitFor(t, 120*time.Second, "investigation outcome to carry both canaries", func() (bool, error) {
			rc, res, err := bc.getInvestigationOutcome(alertNumber)
			if err != nil {
				return false, err
			}
			return strings.Contains(rc, rootCanary) && strings.Contains(res, resCanary), nil
		})
	})

	// resolve_tool and reopen_tool share one fresh alert: the agent may have
	// already resolved the shared alert during its dispatch turn, which would
	// make waiting for "resolved" pass without exercising the mention-driven
	// tool call.
	var resolveAlert int64
	var resolveFP string
	alertResolved := false

	t.Run("resolve_tool", func(t *testing.T) {
		requireTools(t)

		var err error
		resolveAlert, resolveFP, err = bc.createAlert(
			"e2e-agent-resolve-"+runID,
			"warning",
			"Synthetic alert created by the alga-agent E2E test. Briefly acknowledge it in the thread. Do not resolve or close it unless explicitly asked.",
		)
		if err != nil {
			t.Fatalf("create alert: %v", err)
		}
		t.Logf("resolve-scenario alert #%d fingerprint=%s", resolveAlert, resolveFP)

		// Mention forwarding requires the scheduler to have assigned the
		// agent to this alert's investigation first.
		waitForDispatchReply(t, resolveAlert)

		status, err := bc.getAlertStatus(resolveAlert)
		if err != nil {
			t.Fatalf("get alert status: %v", err)
		}
		if status == "resolved" {
			t.Skipf("skipping: model resolved the alert during its dispatch turn despite instructions; cannot assert the mention-driven resolve")
		}

		prompt := fmt.Sprintf("This alert is a synthetic test and is no longer needed. Resolve it now using your resolve tool. Its fingerprint is %s.", resolveFP)
		if err := bc.postThreadMessage(resolveAlert, prompt, []string{"agent:" + tokenID}); err != nil {
			t.Fatalf("post mention: %v", err)
		}
		waitFor(t, 120*time.Second, "alert to be resolved via the agent tool", func() (bool, error) {
			status, err := bc.getAlertStatus(resolveAlert)
			if err != nil {
				return false, err
			}
			return status == "resolved", nil
		})
		alertResolved = true
	})

	t.Run("reopen_tool", func(t *testing.T) {
		requireTools(t)
		if !alertResolved {
			t.Skip("skipping: resolve_tool did not resolve its alert, nothing to reopen")
		}
		prompt := fmt.Sprintf("That alert was resolved by mistake and the issue is still occurring. Reopen it now using your reopen tool. Its fingerprint is %s.", resolveFP)
		if err := bc.postThreadMessage(resolveAlert, prompt, []string{"agent:" + tokenID}); err != nil {
			t.Fatalf("post mention: %v", err)
		}
		waitFor(t, 120*time.Second, "alert to be reopened (firing) via the agent tool", func() (bool, error) {
			status, err := bc.getAlertStatus(resolveAlert)
			if err != nil {
				return false, err
			}
			return status == "firing", nil
		})
	})

	// Runs last: promotion flips the shared alert's investigation to
	// "promoted" and spawns an incident investigation, which would add
	// unrelated agent traffic to earlier scenarios.
	t.Run("promote_tool", func(t *testing.T) {
		requireTools(t)
		title := "E2E-INCIDENT-" + runID
		prompt := fmt.Sprintf("This needs coordinated response. Promote this investigation to an incident now using your promote tool, with the exact title %q and severity SEV2.", title)
		if err := bc.postThreadMessage(alertNumber, prompt, []string{"agent:" + tokenID}); err != nil {
			t.Fatalf("post mention: %v", err)
		}
		var incidentNumber int64
		waitFor(t, 120*time.Second, "an incident titled "+title+" to exist", func() (bool, error) {
			n, found, err := bc.findIncidentByTitle(title)
			if err != nil {
				return false, err
			}
			incidentNumber = n
			return found, nil
		})
		nums, err := bc.incidentAlertNumbers(incidentNumber)
		if err != nil {
			t.Fatalf("list incident alerts: %v", err)
		}
		for _, n := range nums {
			if n == alertNumber {
				return
			}
		}
		t.Errorf("incident #%d is not linked to alert #%d (linked: %v)", incidentNumber, alertNumber, nums)
	})
}
