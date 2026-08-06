package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	alga "github.com/alga/agent-sdk-go"
)

func TestResultHasClaim(t *testing.T) {
	cases := []struct {
		name   string
		result map[string]any
		want   bool
	}{
		{"nil result", nil, false},
		{"no claim keys", map[string]any{"published_status_id": "s1"}, false},
		{"root_cause", map[string]any{"root_cause": "replica lag"}, true},
		{"finding", map[string]any{"finding": "disk full"}, true},
		{"findings list", map[string]any{"findings": []any{"a"}}, true},
		{"root_cause_candidate", map[string]any{"root_cause_candidate": "oom"}, true},
		{"empty claim string", map[string]any{"root_cause": ""}, false},
		{"whitespace claim", map[string]any{"root_cause": "  "}, false},
		{"empty findings list", map[string]any{"findings": []any{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resultHasClaim(tc.result); got != tc.want {
				t.Errorf("resultHasClaim(%v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}

func TestResultHasEvidence(t *testing.T) {
	cases := []struct {
		name   string
		result map[string]any
		want   bool
	}{
		{"nil result", nil, false},
		{"evidence string", map[string]any{"evidence": "pg_stat_replication shows lag=0"}, true},
		{"evidence map", map[string]any{"evidence": map[string]any{"metric": "cpu"}}, true},
		{"evidence list", map[string]any{"evidence": []any{"log line"}}, true},
		{"verification counts", map[string]any{"verification": "pg_isready ok"}, true},
		{"empty evidence string", map[string]any{"evidence": ""}, false},
		{"empty evidence map", map[string]any{"evidence": map[string]any{}}, false},
		{"empty evidence list", map[string]any{"evidence": []any{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resultHasEvidence(tc.result); got != tc.want {
				t.Errorf("resultHasEvidence(%v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}

func TestMissingEvidenceReason(t *testing.T) {
	if reason := missingEvidenceReason(map[string]any{"published_status_id": "s1"}); reason != "" {
		t.Errorf("claim-free result should pass the gate, got %q", reason)
	}
	if reason := missingEvidenceReason(map[string]any{
		"root_cause": "replica lag",
		"evidence":   "pg_stat_replication shows lag=0",
	}); reason != "" {
		t.Errorf("claim with evidence should pass the gate, got %q", reason)
	}
	reason := missingEvidenceReason(map[string]any{"root_cause": "replica lag"})
	if reason == "" {
		t.Fatal("claim without evidence should be rejected")
	}
	if !strings.Contains(reason, "root_cause") || !strings.Contains(reason, "evidence") {
		t.Errorf("reason should name the claim and the missing evidence key: %q", reason)
	}
}

// TestAlgaCompleteTaskEvidenceGate verifies the gate blocks claim-without-
// evidence results in-band (and never reaches the backend), while letting
// evidenced claims and claim-free results through.
func TestAlgaCompleteTaskEvidenceGate(t *testing.T) {
	reg := NewRegistry()
	fc := &fakeAlgaClient{}
	RegisterAlgaTools(reg, fc)

	tool, _ := reg.Get("alga_complete_task")
	ctx := WithCallContext(context.Background(), CallContext{ChatID: "incident_coord_1"})

	t.Run("claim without evidence is rejected", func(t *testing.T) {
		fc.lastCmd = alga.InvestigationCommand{}
		out, err := tool.Execute(ctx, json.RawMessage(`{"task_id":"t1","result":{"root_cause":"replica lag"}}`))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, `"ok":true`) {
			t.Errorf("expected rejection envelope, got %s", out)
		}
		if !strings.Contains(out, "evidence") {
			t.Errorf("rejection should explain the evidence contract, got %s", out)
		}
		if fc.lastCmd.Op != "" {
			t.Errorf("rejected result must not reach the backend, got cmd %+v", fc.lastCmd)
		}
	})

	t.Run("empty evidence is rejected", func(t *testing.T) {
		fc.lastCmd = alga.InvestigationCommand{}
		out, _ := tool.Execute(ctx, json.RawMessage(`{"task_id":"t1","result":{"finding":"disk full","evidence":""}}`))
		if strings.Contains(out, `"ok":true`) {
			t.Errorf("empty evidence should be rejected, got %s", out)
		}
	})

	t.Run("claim with evidence passes", func(t *testing.T) {
		fc.lastCmd = alga.InvestigationCommand{}
		out, err := tool.Execute(ctx, json.RawMessage(`{"task_id":"t1","result":{"root_cause":"replica lag","evidence":"pg_stat_replication shows lag=0","confidence":"observed"}}`))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, `"ok":true`) {
			t.Errorf("expected success envelope, got %s", out)
		}
		if fc.lastCmd.Op != "complete_task" {
			t.Errorf("cmd = %+v", fc.lastCmd)
		}
	})

	t.Run("claim-free result passes without evidence", func(t *testing.T) {
		fc.lastCmd = alga.InvestigationCommand{}
		out, err := tool.Execute(ctx, json.RawMessage(`{"task_id":"t1","result":{"published_status_id":"s1"}}`))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, `"ok":true`) {
			t.Errorf("expected success envelope, got %s", out)
		}
	})
}
