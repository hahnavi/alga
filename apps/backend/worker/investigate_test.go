package worker

import (
	"encoding/json"
	"testing"
	"time"

	"alga/rabbitmq"
)

func TestDedupeKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		msg  rabbitmq.InvestigateMessage
		want string
	}{
		{
			name: "uses_dedupe_key_when_set",
			msg:  rabbitmq.InvestigateMessage{DedupeKey: "flush-abc", InvestigationID: "INV-1", InvestigationKind: rabbitmq.InvestigationKindAlert},
			want: "alga:investigate:dedupe:flush-abc",
		},
		{
			name: "falls_back_to_investigation_id",
			msg:  rabbitmq.InvestigateMessage{InvestigationID: "INV-42", InvestigationKind: rabbitmq.InvestigationKindAlert},
			want: "alga:investigate:dedupe:inv:INV-42",
		},
		{
			name: "empty_when_no_keys",
			msg:  rabbitmq.InvestigateMessage{InvestigationKind: rabbitmq.InvestigationKindAlert},
			want: "",
		},
		{
			name: "dedupe_key_takes_priority",
			msg:  rabbitmq.InvestigateMessage{DedupeKey: "x", InvestigationID: "INV-1", InvestigationKind: rabbitmq.InvestigationKindAlert},
			want: "alga:investigate:dedupe:x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupeKey(tc.msg)
			if got != tc.want {
				t.Fatalf("dedupeKey(%+v) = %q, want %q", tc.msg, got, tc.want)
			}
		})
	}
}

func TestSplitMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		msg    string
		maxLen int
		want   []string
	}{
		{name: "short_message", msg: "hello", maxLen: 100, want: []string{"hello"}},
		{name: "exact_length", msg: "abc", maxLen: 3, want: []string{"abc"}},
		{name: "split_on_newline", msg: "line1\nline2\nline3", maxLen: 8, want: []string{"line1", "\nline2", "\nline3"}},
		{name: "force_split_no_newline", msg: "abcdefghij", maxLen: 4, want: []string{"abcd", "efgh", "ij"}},
		{name: "empty_string", msg: "", maxLen: 10, want: []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitMessage(tc.msg, tc.maxLen)
			if len(got) != len(tc.want) {
				t.Fatalf("splitMessage(%q, %d) returned %d parts, want %d: %+v", tc.msg, tc.maxLen, len(got), len(tc.want), got)
			}
			for i, part := range got {
				if part != tc.want[i] {
					t.Fatalf("splitMessage part[%d] = %q, want %q", i, part, tc.want[i])
				}
			}
		})
	}
}

func TestInvestigateWorkerPrefetchCount(t *testing.T) {
	t.Parallel()
	w := &InvestigateWorker{cfg: InvestigateConfig{MaxConcurrentInvestigations: 5}}
	if got := w.PrefetchCount(); got != 5 {
		t.Fatalf("PrefetchCount() = %d, want 5", got)
	}
}

func TestInvestigateWorkerDefaultConcurrency(t *testing.T) {
	t.Parallel()
	w := NewInvestigateWorker(nil, nil, nil, nil, nil, InvestigateConfig{MaxConcurrentInvestigations: 0})
	if w.cfg.MaxConcurrentInvestigations != 1 {
		t.Fatalf("expected MaxConcurrentInvestigations to be clamped to 1, got %d", w.cfg.MaxConcurrentInvestigations)
	}
}

func TestInvestigateWorkerNegativeConcurrency(t *testing.T) {
	t.Parallel()
	w := NewInvestigateWorker(nil, nil, nil, nil, nil, InvestigateConfig{MaxConcurrentInvestigations: -5})
	if w.cfg.MaxConcurrentInvestigations != 1 {
		t.Fatalf("expected MaxConcurrentInvestigations to be clamped to 1, got %d", w.cfg.MaxConcurrentInvestigations)
	}
}

func TestInvestigateWorkerQueue(t *testing.T) {
	t.Parallel()
	w := &InvestigateWorker{}
	if got := w.Queue(); got != rabbitmq.QueueInvestigateProcess {
		t.Fatalf("Queue() = %q, want %q", got, rabbitmq.QueueInvestigateProcess)
	}
}

func TestTriageEnrichmentToMapOmitsEmptyAndPreservesValues(t *testing.T) {
	got := triageEnrichmentToMap(rabbitmq.TriageEnrichment{
		ServiceOwner:            "payments",
		RunbookURL:              "https://runbooks/payments",
		SuggestedActions:        []string{"check gateway"},
		SimilarInvestigationIDs: []string{"AINV-1"},
		Custom:                  map[string]string{"region": "us-east-1"},
	})

	if got["service_owner"] != "payments" || got["runbook_url"] != "https://runbooks/payments" {
		t.Fatalf("missing scalar enrichment values: %+v", got)
	}
	if actions, ok := got["suggested_actions"].([]string); !ok || len(actions) != 1 || actions[0] != "check gateway" {
		t.Fatalf("suggested_actions = %#v", got["suggested_actions"])
	}
	if similar, ok := got["similar_investigation_ids"].([]string); !ok || len(similar) != 1 || similar[0] != "AINV-1" {
		t.Fatalf("similar_investigation_ids = %#v", got["similar_investigation_ids"])
	}
	if custom, ok := got["custom"].(map[string]string); !ok || custom["region"] != "us-east-1" {
		t.Fatalf("custom = %#v", got["custom"])
	}

	if empty := triageEnrichmentToMap(rabbitmq.TriageEnrichment{}); empty != nil {
		t.Fatalf("empty enrichment = %#v, want nil", empty)
	}
}

func TestInvestigateWorkerSetNotifier(t *testing.T) {
	t.Parallel()
	w := &InvestigateWorker{}
	n := &stubNotifier{}
	w.SetNotifier(n)
	if w.notifier != n {
		t.Fatalf("notifier not set")
	}
}

func TestInvestigateWorkerSetPublisher(t *testing.T) {
	t.Parallel()
	w := &InvestigateWorker{}
	w.SetPublisher(nil)
	if w.publisher != nil {
		t.Fatalf("publisher should be nil")
	}
}

func TestInvestigateMessageRoundTrip(t *testing.T) {
	t.Parallel()
	msg := rabbitmq.InvestigateMessage{
		InvestigationID:   "INV-1",
		InvestigationKind: rabbitmq.InvestigationKindAlert,
		Alerts: []rabbitmq.CorrelatedAlert{
			{Fingerprint: "fp1", Labels: map[string]string{"alertname": "HighCPU"}},
		},
		Severity:       "critical",
		CorrelationKey: "ck-1",
		RetryCount:     0,
		TraceID:        "trace-123",
		DedupeKey:      "dedupe-456",
	}
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded rabbitmq.InvestigateMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.InvestigationID != "INV-1" {
		t.Fatalf("InvestigationID = %q, want %q", decoded.InvestigationID, "INV-1")
	}
	if decoded.Severity != "critical" {
		t.Fatalf("Severity = %q, want %q", decoded.Severity, "critical")
	}
	if decoded.CorrelationKey != "ck-1" {
		t.Fatalf("CorrelationKey = %q, want %q", decoded.CorrelationKey, "ck-1")
	}
	if decoded.TraceID != "trace-123" {
		t.Fatalf("TraceID = %q, want %q", decoded.TraceID, "trace-123")
	}
	if decoded.DedupeKey != "dedupe-456" {
		t.Fatalf("DedupeKey = %q, want %q", decoded.DedupeKey, "dedupe-456")
	}
	if len(decoded.Alerts) != 1 {
		t.Fatalf("len(Alerts) = %d, want 1", len(decoded.Alerts))
	}
	if decoded.InvestigationKind != rabbitmq.InvestigationKindAlert {
		t.Fatalf("InvestigationKind = %q, want %q", decoded.InvestigationKind, rabbitmq.InvestigationKindAlert)
	}
}

func TestInvestigateMessageKindAlert(t *testing.T) {
	t.Parallel()
	msg := rabbitmq.InvestigateMessage{
		InvestigationID:   "AINV-1",
		InvestigationKind: rabbitmq.InvestigationKindAlert,
		Alerts: []rabbitmq.CorrelatedAlert{
			{Fingerprint: "fp1", Labels: map[string]string{"alertname": "TestAlert"}},
		},
		CorrelationKey: "service=test",
	}

	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded rabbitmq.InvestigateMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.InvestigationKind != rabbitmq.InvestigationKindAlert {
		t.Fatalf("InvestigationKind = %q, want %q", decoded.InvestigationKind, rabbitmq.InvestigationKindAlert)
	}
	if !decoded.InvestigationKind.Valid() {
		t.Fatal("InvestigationKind.Valid() returned false for alert kind")
	}
}

func TestInvestigateMessageKindIncident(t *testing.T) {
	t.Parallel()
	msg := rabbitmq.InvestigateMessage{
		InvestigationID:   "IINV-1",
		InvestigationKind: rabbitmq.InvestigationKindIncident,
		IncidentNumber:    1,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded rabbitmq.InvestigateMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.InvestigationKind != rabbitmq.InvestigationKindIncident {
		t.Fatalf("InvestigationKind = %q, want %q", decoded.InvestigationKind, rabbitmq.InvestigationKindIncident)
	}
	if !decoded.InvestigationKind.Valid() {
		t.Fatal("InvestigationKind.Valid() returned false for incident kind")
	}
}

func TestInvestigateMessageInvalidKind(t *testing.T) {
	t.Parallel()
	invalidKind := rabbitmq.InvestigationKind("invalid")
	if invalidKind.Valid() {
		t.Fatal("InvestigationKind.Valid() returned true for invalid kind")
	}
}

func TestDedupeKeyTTL(t *testing.T) {
	t.Parallel()
	if dedupeKeyTTL != 24*time.Hour {
		t.Fatalf("dedupeKeyTTL = %v, want 24h", dedupeKeyTTL)
	}
}

type stubNotifier struct {
	callCount int
}

func (n *stubNotifier) NotifyPending() {
	n.callCount++
}
