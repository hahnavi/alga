package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMultiAgentCoordination drives a full incident coordination flow with two
// agents: a commander (command capability) and a responder (investigate
// capability). It asserts ICS role assignment, coordination task dispatch and
// completion, and incident lifecycle transitions.
//
// Gated behind ALGA_AGENT_E2E=1 (stack) and ALGA_AGENT_E2E_COORDINATION=1
// (this test specifically) because it runs two LLM agents and is highly
// model-dependent.
func TestMultiAgentCoordination(t *testing.T) {
	serverURL := requireE2E(t)
	if os.Getenv("ALGA_AGENT_E2E_COORDINATION") != "1" {
		t.Skip("skipping: set ALGA_AGENT_E2E_COORDINATION=1 to run the multi-agent coordination test")
	}
	cfg := loadAgentConfig(t, serverURL)
	runID := fmt.Sprintf("%d", time.Now().UnixNano())

	bc, err := newBackendClient(serverURL)
	if err != nil {
		t.Fatalf("backend client: %v", err)
	}
	if err := bc.setupOrLogin("e2e-admin@alga.local", e2eAdminPassword(), "E2E Admin"); err != nil {
		t.Fatalf("setup/login: %v", err)
	}

	commanderID, commanderToken, err := bc.mintAgentToken("e2e-commander-"+runID, "command")
	if err != nil {
		t.Fatalf("mint commander token: %v", err)
	}
	responderID, responderToken, err := bc.mintAgentToken("e2e-responder-"+runID, "investigate")
	if err != nil {
		t.Fatalf("mint responder token: %v", err)
	}
	t.Logf("commander token id: %s", commanderID)
	t.Logf("responder token id: %s", responderID)

	startAgent(t, cfg, commanderToken)
	startAgent(t, cfg, responderToken)

	waitFor(t, 30*time.Second, "commander agent online", func() (bool, error) {
		return bc.agentOnline(commanderID)
	})
	waitFor(t, 30*time.Second, "responder agent online", func() (bool, error) {
		return bc.agentOnline(responderID)
	})

	incidentTitle := "E2E-COORD-" + runID
	incidentNumber, err := bc.createIncident(incidentTitle, "high")
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	t.Logf("incident #%d created", incidentNumber)

	if err := bc.acknowledgeIncident(incidentNumber); err != nil {
		t.Fatalf("acknowledge incident: %v", err)
	}

	t.Run("ics_role_assignment", func(t *testing.T) {
		waitFor(t, 60*time.Second, "commander to hold incident_commander role", func() (bool, error) {
			return bc.hasActiveRole(incidentNumber, "incident_commander", commanderID)
		})
		waitFor(t, 60*time.Second, "responder to hold responder role", func() (bool, error) {
			return bc.hasActiveRole(incidentNumber, "responder", responderID)
		})
	})

	canary := "E2E-TASK-" + runID
	dispatchOK := t.Run("commander_dispatches_task", func(t *testing.T) {
		prompt := fmt.Sprintf(
			"You are the incident commander for incident #%d. "+
				"Dispatch a coordination task now using alga_dispatch_task with kind \"investigate\", "+
				"assignee_role \"responder\", and goal exactly: %q. "+
				"Do not call any other tools.",
			incidentNumber, canary,
		)
		if err := bc.postIncidentThreadMessage(incidentNumber, prompt, []string{"agent:" + commanderID}); err != nil {
			t.Fatalf("post commander mention: %v", err)
		}
		waitFor(t, 180*time.Second, "a coordination task with canary goal to exist", func() (bool, error) {
			tasks, err := bc.listCoordinationTasks(incidentNumber)
			if err != nil {
				return false, err
			}
			for _, task := range tasks {
				if strings.Contains(task.Goal, canary) {
					return true, nil
				}
			}
			return false, nil
		})
	})

	t.Run("responder_completes_task", func(t *testing.T) {
		if !dispatchOK {
			t.Skip("skipping: commander_dispatches_task failed")
		}
		waitFor(t, 180*time.Second, "coordination task to reach complete status", func() (bool, error) {
			tasks, err := bc.listCoordinationTasks(incidentNumber)
			if err != nil {
				return false, err
			}
			for _, task := range tasks {
				if strings.Contains(task.Goal, canary) && task.Status == "complete" {
					return true, nil
				}
			}
			return false, nil
		})
	})

	t.Run("commander_mitigates", func(t *testing.T) {
		if !dispatchOK {
			t.Skip("skipping: commander_dispatches_task failed")
		}
		prompt := fmt.Sprintf(
			"The investigation is complete and the issue is contained. "+
				"Mitigate incident #%d now using alga_mitigate_incident with reason %q. "+
				"Do not call any other tools.",
			incidentNumber, "E2E mitigation "+runID,
		)
		if err := bc.postIncidentThreadMessage(incidentNumber, prompt, []string{"agent:" + commanderID}); err != nil {
			t.Fatalf("post commander mention: %v", err)
		}
		waitFor(t, 120*time.Second, "incident to be mitigated", func() (bool, error) {
			status, err := bc.getIncidentStatus(incidentNumber)
			if err != nil {
				return false, err
			}
			return status == "mitigated", nil
		})
	})

	t.Run("commander_resolves", func(t *testing.T) {
		status, err := bc.getIncidentStatus(incidentNumber)
		if err != nil {
			t.Fatalf("get incident status: %v", err)
		}
		if status != "mitigated" {
			t.Skipf("skipping: incident is %q, expected mitigated", status)
		}
		prompt := fmt.Sprintf(
			"The incident is fully resolved. First set the resolution documents using alga_set_incident_resolution_docs "+
				"with summary %q, root_cause %q, and resolution %q. "+
				"Then resolve incident #%d using alga_resolve_incident with reason %q.",
			"E2E summary "+runID, "E2E root cause "+runID, "E2E resolution "+runID,
			incidentNumber, "E2E resolution "+runID,
		)
		if err := bc.postIncidentThreadMessage(incidentNumber, prompt, []string{"agent:" + commanderID}); err != nil {
			t.Fatalf("post commander mention: %v", err)
		}
		waitFor(t, 180*time.Second, "incident to be resolved", func() (bool, error) {
			status, err := bc.getIncidentStatus(incidentNumber)
			if err != nil {
				return false, err
			}
			return status == "resolved", nil
		})
	})
}
