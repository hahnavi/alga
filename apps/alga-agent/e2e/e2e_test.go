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
// scheduler having assigned the agent during the dispatch flow.
func TestAlgaAgentE2E(t *testing.T) {
	serverURL := requireE2E(t)
	cfg := loadAgentConfig(t, serverURL)
	runID := fmt.Sprintf("%d", time.Now().UnixNano())

	bc, err := newBackendClient(serverURL)
	if err != nil {
		t.Fatalf("backend client: %v", err)
	}
	if err := bc.setupOrLogin("e2e-admin@alga.local", "E2eAdmin!12345", "E2E Admin"); err != nil {
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

	t.Run("resolve_tool", func(t *testing.T) {
		if os.Getenv("ALGA_AGENT_E2E_TOOLS") != "1" {
			t.Skip("skipping: set ALGA_AGENT_E2E_TOOLS=1 to test tool effects (model-dependent, may flake)")
		}
		if !dispatchOK {
			t.Skip("skipping: dispatch_reply failed, dispatch pipeline is not working")
		}

		// Use a fresh alert: the agent may have already resolved the shared
		// alert during its dispatch turn, which would make waiting for
		// "resolved" pass without exercising the mention-driven tool call.
		resolveAlert, resolveFP, err := bc.createAlert(
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
	})
}
