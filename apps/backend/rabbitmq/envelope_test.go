package rabbitmq

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEnsureEnvelopePopulatesMissingFields(t *testing.T) {
	t.Parallel()
	var e EventEnvelope
	e.ensureEnvelope(EventTypeAlertReceived, DefaultEventVersion, "trace-abc")

	if e.EventID == "" {
		t.Fatal("EventID was not generated")
	}
	if e.EventType != EventTypeAlertReceived {
		t.Fatalf("EventType = %q, want %q", e.EventType, EventTypeAlertReceived)
	}
	if e.EventVersion != DefaultEventVersion {
		t.Fatalf("EventVersion = %d, want %d", e.EventVersion, DefaultEventVersion)
	}
	if e.OccurredAt.IsZero() {
		t.Fatal("OccurredAt was not set")
	}
	if e.TraceID != "trace-abc" {
		t.Fatalf("TraceID = %q, want %q", e.TraceID, "trace-abc")
	}
}

func TestEnsureEnvelopeIsIdempotent(t *testing.T) {
	t.Parallel()
	occurred := time.Now().Add(-time.Hour).UTC()
	e := EventEnvelope{
		EventID:      "existing-id",
		EventType:    EventTypeIncidentPromoted,
		EventVersion: 3,
		OccurredAt:   occurred,
		TraceID:      "existing-trace",
	}
	// Simulate a re-publish onto a retry queue: nothing must change.
	e.ensureEnvelope(EventTypeAlertReceived, DefaultEventVersion, "new-trace")

	if e.EventID != "existing-id" {
		t.Fatalf("EventID = %q, want preserved %q", e.EventID, "existing-id")
	}
	if e.EventType != EventTypeIncidentPromoted {
		t.Fatalf("EventType = %q, want preserved %q", e.EventType, EventTypeIncidentPromoted)
	}
	if e.EventVersion != 3 {
		t.Fatalf("EventVersion = %d, want preserved 3", e.EventVersion)
	}
	if !e.OccurredAt.Equal(occurred) {
		t.Fatalf("OccurredAt = %s, want preserved %s", e.OccurredAt, occurred)
	}
	if e.TraceID != "existing-trace" {
		t.Fatalf("TraceID = %q, want preserved %q", e.TraceID, "existing-trace")
	}
}

func TestEventTypesUseAggregateActionNaming(t *testing.T) {
	t.Parallel()
	types := []string{
		EventTypeAlertReceived,
		EventTypeTriageRequested,
		EventTypeInvestigationRequested,
		EventTypeEmailRequested,
		EventTypeIncidentPromoted,
		EventTypeEscalationTriggered,
		EventTypeSLASweepRequested,
		EventTypeNotificationDispatched,
		EventTypeICSProvisionRequested,
	}
	for _, et := range types {
		if et != strings.ToLower(et) {
			t.Errorf("event type %q must be lowercase", et)
		}
		aggregate, action, ok := strings.Cut(et, ".")
		if !ok || aggregate == "" || action == "" {
			t.Errorf("event type %q must be of the form aggregate.action", et)
		}
	}
}

// TestEveryMessageTypeCarriesEnvelope confirms the shared envelope is embedded
// (and thus JSON-serialized) by every message type Alga publishes.
func TestEveryMessageTypeCarriesEnvelope(t *testing.T) {
	t.Parallel()
	messages := []any{
		&AlertMessage{},
		&InvestigateMessage{},
		&EmailMessage{},
		&TriageMessage{},
		&IncidentMessage{},
		&EscalationMessage{},
		&SLASweepMessage{},
		&NotificationDispatchMessage{},
		&ICSProvisionMessage{},
	}
	for _, m := range messages {
		carrier, ok := m.(envelopeCarrier)
		if !ok {
			t.Fatalf("%T does not embed EventEnvelope", m)
		}
		carrier.ensureEnvelope("test.event", DefaultEventVersion, "trace-1")

		body, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal %T: %v", m, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("unmarshal %T: %v", m, err)
		}
		if _, present := decoded["event_id"]; !present {
			t.Errorf("%T serialized without event_id: %s", m, body)
		}
		if decoded["event_type"] != "test.event" {
			t.Errorf("%T event_type = %v, want test.event", m, decoded["event_type"])
		}
	}
}
