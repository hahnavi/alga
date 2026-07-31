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
// and share one alert and agent token — the mention and resolve flows depend
// on the scheduler having assigned the agent during the dispatch flow.
func TestAlgaAgentE2E(t *testing.T) {
	serverURL := requireE2E(t)
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

	startAgent(t, serverURL, token)

	waitFor(t, 30*time.Second, "agent to come online (live SSE)", func() (bool, error) {
		return bc.agentOnline(tokenID)
	})

	alertNumber, fingerprint, err := bc.createAlert(
		"e2e-agent-"+runID,
		"critical",
		"Synthetic alert created by the alga-agent E2E test. Briefly acknowledge it in the thread.",
	)
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}
	t.Logf("alert #%d fingerprint=%s", alertNumber, fingerprint)

	agentMessages := func() ([]threadMessage, error) {
		msgs, err := bc.getThread(alertNumber)
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

	t.Run("dispatch_reply", func(t *testing.T) {
		waitFor(t, 120*time.Second, "scheduler dispatch and an agent reply in the alert thread", func() (bool, error) {
			msgs, err := agentMessages()
			if err != nil {
				return false, err
			}
			return len(msgs) > 0, nil
		})
	})

	t.Run("mention_reply", func(t *testing.T) {
		base, err := agentMessages()
		if err != nil {
			t.Fatalf("read thread: %v", err)
		}
		canary := "E2E-CANARY-" + runID
		prompt := fmt.Sprintf("Please reply with exactly the marker %s and nothing else. Do not call any tools.", canary)
		if err := bc.postThreadMessage(alertNumber, prompt, []string{"agent:" + tokenID}); err != nil {
			t.Fatalf("post mention: %v", err)
		}
		waitFor(t, 90*time.Second, "a new agent reply to the mention", func() (bool, error) {
			msgs, err := agentMessages()
			if err != nil {
				return false, err
			}
			return len(msgs) > len(base), nil
		})
		msgs, err := agentMessages()
		if err != nil {
			t.Fatalf("read thread: %v", err)
		}
		var all strings.Builder
		for _, m := range msgs {
			all.WriteString(m.Message)
			all.WriteString("\n")
		}
		if !strings.Contains(all.String(), canary) {
			t.Errorf("agent replies do not contain canary %q; replies:\n%s", canary, all.String())
		}
	})

	t.Run("resolve_tool", func(t *testing.T) {
		if os.Getenv("ALGA_AGENT_E2E_TOOLS") != "1" {
			t.Skip("skipping: set ALGA_AGENT_E2E_TOOLS=1 to test tool effects (model-dependent, may flake)")
		}
		prompt := fmt.Sprintf("This alert is a synthetic test and is no longer needed. Resolve it now using your resolve tool. Its fingerprint is %s.", fingerprint)
		if err := bc.postThreadMessage(alertNumber, prompt, []string{"agent:" + tokenID}); err != nil {
			t.Fatalf("post mention: %v", err)
		}
		waitFor(t, 120*time.Second, "alert to be resolved via the agent tool", func() (bool, error) {
			status, err := bc.getAlertStatus(alertNumber)
			if err != nil {
				return false, err
			}
			return status == "resolved", nil
		})
	})
}
