package alga

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestInvestigationCommandJSON verifies the wire shape of every command builder
// against what the backend's InvTool struct expects. This guards the
// incident_number-vs-incident_id regression and the scope_description pointer.
func TestInvestigationCommandJSON(t *testing.T) {
	cases := []struct {
		name string
		cmd  InvestigationCommand
		want map[string]any
	}{
		{
			name: "resolve_alert",
			cmd:  ResolveAlert("fp123"),
			want: map[string]any{"op": "resolve_alert", "fingerprint": "fp123"},
		},
		{
			name: "set_outcome",
			cmd:  SetOutcome(StrPtr("rc"), StrPtr("res")),
			want: map[string]any{"op": "set_outcome", "root_cause": "rc", "resolution": "res"},
		},
		{
			name: "set_incident_priority",
			cmd:  SetIncidentPriority(42, "high"),
			want: map[string]any{"op": "set_incident_priority", "incident_number": float64(42), "priority": "high"},
		},
		{
			name: "trigger_escalation",
			cmd:  TriggerEscalation(7),
			want: map[string]any{"op": "trigger_escalation", "incident_number": float64(7)},
		},
		{
			name: "mitigate_incident",
			cmd:  MitigateIncident(7, "patched"),
			want: map[string]any{"op": "mitigate_incident", "incident_number": float64(7), "reason": "patched"},
		},
		{
			name: "assign_incident_role to user",
			cmd:  AssignIncidentRoleToUser(3, "commander", "user-1", "write"),
			want: map[string]any{
				"op":                "assign_incident_role",
				"incident_number":   float64(3),
				"role_type":         "commander",
				"user_id":           "user-1",
				"scope_description": "write",
			},
		},
		{
			name: "assign_incident_role to agent",
			cmd:  AssignIncidentRoleToAgent(3, "communicator", "tok-1", ""),
			want: map[string]any{
				"op":              "assign_incident_role",
				"incident_number": float64(3),
				"role_type":       "communicator",
				"agent_token_id":  "tok-1",
			},
		},
		{
			name: "post_handoff",
			cmd:  PostHandoff(11, "handing off", "commander", "info"),
			want: map[string]any{
				"op":              "post_handoff",
				"incident_number": float64(11),
				"message":         "handing off",
				"audience":        "commander",
				"urgency":         "info",
			},
		},
		{
			name: "publish_status_update",
			cmd:  PublishStatusUpdate(11, "root cause found", "identified"),
			want: map[string]any{
				"op":              "publish_status_update",
				"incident_number": float64(11),
				"message":         "root cause found",
				"status_level":    "identified",
			},
		},
		{
			name: "set_incident_resolution_docs",
			cmd:  SetIncidentResolutionDocs(11, "s", "ia", "at", "rc", "res"),
			want: map[string]any{
				"op":                "set_incident_resolution_docs",
				"incident_number":   float64(11),
				"summary":           "s",
				"impact_assessment": "ia",
				"actions_taken":     "at",
				"root_cause":        "rc",
				"resolution":        "res",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.cmd)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("field %q = %v, want %v (full: %s)", k, got[k], want, raw)
				}
			}
			// incident commands must NEVER serialize an incident_id key.
			if _, ok := got["incident_id"]; ok {
				t.Errorf("command serialized banned field incident_id: %s", raw)
			}
		})
	}
}

// TestAssignIncidentRoleExactlyOneAssignee locks in the backend rule that
// exactly one of user_id / agent_token_id is present per assignment, and that
// an empty scope description is omitted rather than serialized as "".
func TestAssignIncidentRoleExactlyOneAssignee(t *testing.T) {
	raw, _ := json.Marshal(AssignIncidentRoleToUser(1, "commander", "u1", ""))
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if _, ok := got["agent_token_id"]; ok {
		t.Errorf("user assignment leaked agent_token_id: %s", raw)
	}
	if _, ok := got["scope_description"]; ok {
		t.Errorf("empty scope_description must be omitted: %s", raw)
	}

	raw, _ = json.Marshal(AssignIncidentRoleToAgent(1, "commander", "t1", ""))
	got = map[string]any{}
	_ = json.Unmarshal(raw, &got)
	if _, ok := got["user_id"]; ok {
		t.Errorf("agent assignment leaked user_id: %s", raw)
	}
}

func TestMessageDedupBasic(t *testing.T) {
	d := NewMessageDedup(100, time.Minute)
	if d.IsDuplicate("a") {
		t.Fatal("first sighting should not be a duplicate")
	}
	if !d.IsDuplicate("a") {
		t.Fatal("second sighting of same id should be a duplicate")
	}
	if d.IsDuplicate("b") {
		t.Fatal("first sighting of a new id should not be a duplicate")
	}
}

// TestMessageDedupNoEvictOnInsert reproduces the original bug where a
// just-accepted ID could be evicted by its own insertion when the cache was at
// capacity, letting an immediate replay be treated as new.
func TestMessageDedupNoEvictOnInsert(t *testing.T) {
	d := NewMessageDedup(3, time.Minute)
	// Fill to capacity.
	for i := 0; i < 3; i++ {
		d.IsDuplicate("id-" + string(rune('a'+i)))
	}
	// At capacity: the next unique insert must still be recorded, and an
	// immediate replay must be detected as a duplicate (not evicted).
	if d.IsDuplicate("id-new") {
		t.Fatal("first sighting of id-new should not be a duplicate")
	}
	if !d.IsDuplicate("id-new") {
		t.Fatal("id-new was evicted by its own insertion — replay not detected")
	}
}

func TestMessageDedupTTLExpiry(t *testing.T) {
	d := NewMessageDedup(100, 20*time.Millisecond)
	d.IsDuplicate("ephemeral")
	time.Sleep(30 * time.Millisecond)
	if d.IsDuplicate("ephemeral") {
		t.Fatal("id should have expired and been re-accepted")
	}
}

// TestCoordinationCommandJSON locks in the wire shape for the coordination
// task subsystem (dispatch_task / claim_task / complete_task /
// synthesize_findings). These mirror the backend InvTool fields exactly so
// the backend's op dispatch accepts them.
func TestCoordinationCommandJSON(t *testing.T) {
	cases := []struct {
		name string
		cmd  InvestigationCommand
		want map[string]any
	}{
		{
			name: "dispatch_task by role",
			cmd:  DispatchTask(99, TaskKindInvestigate, "find root cause", "responder"),
			want: map[string]any{
				"op":              "dispatch_task",
				"incident_number": float64(99),
				"task_kind":       "investigate",
				"goal":            "find root cause",
				"assignee_role":   "responder",
			},
		},
		{
			name: "dispatch_task by agent",
			cmd:  DispatchTaskToAgent(99, TaskKindCommunicate, "publish status", "agent-42"),
			want: map[string]any{
				"op":                "dispatch_task",
				"incident_number":   float64(99),
				"task_kind":         "communicate",
				"goal":              "publish status",
				"assignee_agent_id": "agent-42",
			},
		},
		{
			name: "claim_task",
			cmd:  ClaimTask("task-7"),
			want: map[string]any{"op": "claim_task", "task_id": "task-7"},
		},
		{
			name: "complete_task",
			cmd:  CompleteTask("task-7", map[string]any{"finding": "leaky bucket"}),
			want: map[string]any{
				"op":      "complete_task",
				"task_id": "task-7",
				"result":  map[string]any{"finding": "leaky bucket"},
			},
		},
		{
			name: "synthesize_findings",
			cmd:  SynthesizeFindings(7, "combined rc", map[string]any{"evidence_count": float64(3)}),
			want: map[string]any{
				"op":              "synthesize_findings",
				"incident_number": float64(7),
				"result": map[string]any{
					"summary":        "combined rc",
					"evidence_count": float64(3),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.cmd)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			for k, want := range tc.want {
				if !deepEqualJSON(got[k], want) {
					t.Errorf("field %q = %v (%T), want %v (%T) [raw=%s]", k, got[k], got[k], want, want, raw)
				}
			}
		})
	}
}

// deepEqualJSON compares values that have been round-tripped through
// encoding/json, where numbers become float64 and nested objects become
// map[string]any.
func deepEqualJSON(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqualJSON(v, bv[k]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// TestUnmarshalData covers the backend response envelope handling: standard
// {"data": ...} responses are unwrapped, flat bodies decode as-is, and a JSON
// null data field falls back to the flat path.
func TestUnmarshalData(t *testing.T) {
	t.Run("enveloped list", func(t *testing.T) {
		var out []Alert
		if err := unmarshalData([]byte(`{"data":[{"fingerprint":"fp1"}]}`), &out); err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || out[0].Fingerprint != "fp1" {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("enveloped paginated", func(t *testing.T) {
		var out KnowledgeListResponse
		raw := `{"data":{"items":[{"id":"k1","kind":"runbook","title":"t","body_markdown":"b","author_type":"agent"}],"total":1},"meta":{"total":1}}`
		if err := unmarshalData([]byte(raw), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Items) != 1 || out.Items[0].ID != "k1" || out.Total != 1 {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("flat body", func(t *testing.T) {
		var out SendMessageResponse
		if err := unmarshalData([]byte(`{"status":"ok","message_id":"m1"}`), &out); err != nil {
			t.Fatal(err)
		}
		if out.MessageID != "m1" {
			t.Fatalf("out = %+v", out)
		}
	})
	t.Run("null data falls back to flat", func(t *testing.T) {
		var out map[string]any
		if err := unmarshalData([]byte(`{"data":null,"status":"ok"}`), &out); err != nil {
			t.Fatal(err)
		}
		if out["status"] != "ok" {
			t.Fatalf("out = %+v", out)
		}
	})
}

// TestErrorClassification covers the retryable classifications used by the
// REST retry loop.
func TestErrorClassification(t *testing.T) {
	if !(&AlgaAPIError{StatusCode: 429}).IsRetryable() {
		t.Error("429 should be retryable")
	}
	if !(&AlgaAPIError{StatusCode: 503}).IsRetryable() {
		t.Error("503 should be retryable")
	}
	if (&AlgaAPIError{StatusCode: 400}).IsRetryable() {
		t.Error("400 should not be retryable")
	}
	if (&AlgaAPIError{StatusCode: 404}).IsRetryable() {
		t.Error("404 should not be retryable")
	}
	if !IsRetryableError(&AlgaAPIError{StatusCode: 502}) {
		t.Error("IsRetryableError(502) = false, want true")
	}
	if IsRetryableError(&AlgaAPIError{StatusCode: 401}) {
		t.Error("IsRetryableError(401) = true, want false")
	}
	if IsRetryableError(&AlgaAuthError{StatusCode: 401}) {
		t.Error("auth errors are never retryable")
	}
	// Connection errors: caller-initiated cancellation is never retryable;
	// transport resets are.
	if (&AlgaConnectionError{Err: context.Canceled}).IsRetryable() {
		t.Error("context.Canceled should not be retryable")
	}
	if (&AlgaConnectionError{Err: context.DeadlineExceeded}).IsRetryable() {
		t.Error("context.DeadlineExceeded should not be retryable")
	}
	if !(&AlgaConnectionError{Err: errFake}).IsRetryable() {
		t.Error("generic transport error should be retryable")
	}
}

var errFake = &fakeNetErr{}

type fakeNetErr struct{}

func (*fakeNetErr) Error() string { return "connection reset by peer" }

// TestBackoffForMonotonic sanity-checks that backoff grows (modulo jitter)
// and honors Retry-After when provided.
func TestBackoffForMonotonic(t *testing.T) {
	for attempt := 0; attempt < 5; attempt++ {
		d := backoffFor(attempt, 0)
		if d > 37*time.Second {
			t.Errorf("backoff attempt %d = %s, must be capped near 30s + jitter", attempt, d)
		}
		base := time.Second * time.Duration(int64(1)<<attempt)
		base = min(base, 30*time.Second)
		if d < base {
			t.Errorf("backoff attempt %d = %s, must be >= base %s", attempt, d, base)
		}
	}
	if got := backoffFor(0, 5*time.Second); got != 5*time.Second {
		t.Errorf("Retry-After override = %s, want 5s", got)
	}
	if got := backoffFor(0, 0); got == 0 {
		t.Error("backoff with no retry-after must be non-zero")
	}
}

// TestOptionsDefaults verifies the With* defaults applied when no option is
// supplied.
func TestOptionsDefaults(t *testing.T) {
	o := &Options{}
	defaults(o)
	if o.HTTPClient == nil {
		t.Error("HTTPClient should default to non-nil")
	}
	if o.Logger == nil {
		t.Error("Logger should default to slog.Default()")
	}
	if o.Dedup == nil {
		t.Error("Dedup should default to a fresh cache")
	}
	if o.UserAgent == "" {
		t.Error("UserAgent should default to a non-empty string")
	}
	if o.MaxRESTRetries != 2 {
		t.Errorf("MaxRESTRetries default = %d, want 2", o.MaxRESTRetries)
	}
	if o.HeartbeatInterval != 30*time.Second {
		t.Errorf("HeartbeatInterval default = %s, want 30s", o.HeartbeatInterval)
	}
}

// TestHeartbeatIntervalClamp verifies that sub-1s intervals are clamped to
// protect the backend.
func TestHeartbeatIntervalClamp(t *testing.T) {
	o := &Options{HeartbeatInterval: 1 * time.Millisecond}
	defaults(o)
	if o.HeartbeatInterval != time.Second {
		t.Errorf("HeartbeatInterval = %s, want clamped 1s", o.HeartbeatInterval)
	}
}
