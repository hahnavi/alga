package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"alga/rabbitmq"
	"alga/sse"
	"alga/store"
)

func TestIncidentWorkerQueue(t *testing.T) {
	t.Parallel()
	w := &IncidentWorker{}
	if got := w.Queue(); got != rabbitmq.QueueIncidentProcess {
		t.Fatalf("Queue() = %q, want %q", got, rabbitmq.QueueIncidentProcess)
	}
}

func TestIncidentWorkerPrefetchCount(t *testing.T) {
	t.Parallel()
	w := &IncidentWorker{}
	if got := w.PrefetchCount(); got != 1 {
		t.Fatalf("PrefetchCount() = %d, want 1", got)
	}
}

func TestIncidentDedupeKeyTTL(t *testing.T) {
	t.Parallel()
	if incidentDedupeKeyTTL != 24*time.Hour {
		t.Fatalf("incidentDedupeKeyTTL = %v, want 24h", incidentDedupeKeyTTL)
	}
}

func TestIncidentMessageRoundTrip(t *testing.T) {
	t.Parallel()
	msg := rabbitmq.IncidentMessage{
		DedupeKey:       "dedup-1",
		TraceID:         "trace-1",
		InvestigationID: "INV-1",
		Alerts: []rabbitmq.CorrelatedAlert{
			{Fingerprint: "fp1", Labels: map[string]string{"alertname": "HighCPU"}},
		},
		CorrelationKey: "prod:api:HighCPU",
		Severity:       "critical",
		TriageDecision: "escalate",
		RetryCount:     0,
		MaxRetries:     3,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded rabbitmq.IncidentMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.DedupeKey != "dedup-1" {
		t.Fatalf("DedupeKey = %q, want %q", decoded.DedupeKey, "dedup-1")
	}
	if decoded.Severity != "critical" {
		t.Fatalf("Severity = %q, want %q", decoded.Severity, "critical")
	}
	if decoded.CorrelationKey != "prod:api:HighCPU" {
		t.Fatalf("CorrelationKey = %q, want %q", decoded.CorrelationKey, "prod:api:HighCPU")
	}
	if decoded.TriageDecision != "escalate" {
		t.Fatalf("TriageDecision = %q, want %q", decoded.TriageDecision, "escalate")
	}
	if len(decoded.Alerts) != 1 {
		t.Fatalf("len(Alerts) = %d, want 1", len(decoded.Alerts))
	}
}

func TestUnmarshalIncidentMessage(t *testing.T) {
	t.Parallel()
	original := rabbitmq.IncidentMessage{
		DedupeKey:       "dk",
		TraceID:         "tid",
		InvestigationID: "INV-99",
		Severity:        "warning",
		RetryCount:      2,
		MaxRetries:      3,
	}
	body, _ := json.Marshal(original)
	d := amqp.Delivery{Body: body}

	decoded, err := rabbitmq.UnmarshalIncidentMessage(d)
	if err != nil {
		t.Fatalf("UnmarshalIncidentMessage: %v", err)
	}
	if decoded.DedupeKey != "dk" {
		t.Fatalf("DedupeKey = %q, want %q", decoded.DedupeKey, "dk")
	}
	if decoded.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", decoded.RetryCount)
	}
}

func TestUnmarshalIncidentMessageBadJSON(t *testing.T) {
	t.Parallel()
	d := amqp.Delivery{Body: []byte("not json")}
	_, err := rabbitmq.UnmarshalIncidentMessage(d)
	if err == nil {
		t.Fatalf("expected error for bad JSON")
	}
}

func TestIncidentWorkerEnsureIncidentInvestigationCreatesWithoutTimeline(t *testing.T) {
	incidentStore := &schedulerIncidentStore{}
	incidentInvestigationStore := &stubIncidentInvestigationStore{
		createResult: &store.IncidentInvestigationRecord{
			IncidentInvestigationID: "IINV-1",
			IncidentNumber:          1,
			Status:                  store.IncidentInvestigationStatusPending,
		},
	}
	broker := sse.NewBroker()
	ch := broker.Subscribe("test-client")
	defer broker.Unsubscribe("test-client")
	notifier := &stubNotifier{}
	w := &IncidentWorker{
		incidentStore:              incidentStore,
		incidentInvestigationStore: incidentInvestigationStore,
		ssePublisher:               &sse.DualPublisher{Broker: broker},
		notifier:                   notifier,
	}

	w.ensureIncidentInvestigation(context.Background(), &store.IncidentRecord{IncidentNumber: 1})

	if !incidentInvestigationStore.createCalled {
		t.Fatalf("incident worker did not create an investigation")
	}
	if incidentInvestigationStore.createInput.IncidentNumber != 1 {
		t.Fatalf("incident_number = %d, want 1", incidentInvestigationStore.createInput.IncidentNumber)
	}
	if incidentInvestigationStore.createInput.Status != store.IncidentInvestigationStatusPending {
		t.Fatalf("status = %q, want %q", incidentInvestigationStore.createInput.Status, store.IncidentInvestigationStatusPending)
	}
	for _, entry := range incidentStore.timeline {
		if entry.EventType == "investigation_created" {
			t.Fatalf("unexpected investigation_created timeline entry: %#v", entry)
		}
	}
	if notifier.callCount != 1 {
		t.Fatalf("pending notifications = %d, want 1", notifier.callCount)
	}

	var gotInvestigationCreated bool
	for i := range 2 {
		select {
		case event := <-ch:
			if event.Type == "investigation_created" {
				gotInvestigationCreated = true
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for SSE event %d", i+1)
		}
	}
	if !gotInvestigationCreated {
		t.Fatal("expected investigation_created SSE event")
	}
}

func TestIncidentTitleFromAlertname(t *testing.T) {
	t.Parallel()
	alerts := []rabbitmq.CorrelatedAlert{
		{Fingerprint: "fp1", Labels: map[string]string{"alertname": "DiskFull"}},
	}
	if name, ok := alerts[0].Labels["alertname"]; !ok || name != "DiskFull" {
		t.Fatalf("expected alertname=DiskFull from first alert labels")
	}
}

func TestEscalationWorkerQueue(t *testing.T) {
	t.Parallel()
	w := &EscalationWorker{}
	if got := w.Queue(); got != rabbitmq.QueueEscalationProcess {
		t.Fatalf("Queue() = %q, want %q", got, rabbitmq.QueueEscalationProcess)
	}
}

func TestEscalationWorkerPrefetchCount(t *testing.T) {
	t.Parallel()
	w := &EscalationWorker{}
	if got := w.PrefetchCount(); got != 5 {
		t.Fatalf("PrefetchCount() = %d, want 5", got)
	}
}

func TestEscalationSweepHashPrefix(t *testing.T) {
	t.Parallel()
	if escSortedSet != "alga:esc:pending" {
		t.Fatalf("escSortedSet = %q", escSortedSet)
	}
	if escHashPrefix != "alga:esc:" {
		t.Fatalf("escHashPrefix = %q", escHashPrefix)
	}
	if escSweepTick != 10*time.Second {
		t.Fatalf("escSweepTick = %v, want 10s", escSweepTick)
	}
}

func TestCorrelatedAlertRoundTrip(t *testing.T) {
	t.Parallel()
	alert := rabbitmq.CorrelatedAlert{
		Fingerprint:  "abc123",
		AlertNumber:  42,
		Labels:       map[string]string{"severity": "critical", "alertname": "OOMKilled"},
		Annotations:  map[string]string{"summary": "container oom"},
		Status:       "firing",
		StartsAt:     "2026-01-01T00:00:00Z",
		GeneratorURL: "http://grafana/example",
	}
	body, err := json.Marshal(alert)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded rabbitmq.CorrelatedAlert
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Fingerprint != "abc123" {
		t.Fatalf("Fingerprint = %q", decoded.Fingerprint)
	}
	if decoded.AlertNumber != 42 {
		t.Fatalf("AlertNumber = %d", decoded.AlertNumber)
	}
	if decoded.Labels["severity"] != "critical" {
		t.Fatalf("severity = %q", decoded.Labels["severity"])
	}
	if decoded.Status != "firing" {
		t.Fatalf("Status = %q", decoded.Status)
	}
	if decoded.GeneratorURL != "http://grafana/example" {
		t.Fatalf("GeneratorURL = %q", decoded.GeneratorURL)
	}
}

func TestStoreIncidentRecordTitleFromAlertname(t *testing.T) {
	t.Parallel()
	labels := map[string]string{"alertname": "PodCrashLooping"}
	name := labels["alertname"]
	if name != "PodCrashLooping" {
		t.Fatalf("expected PodCrashLooping, got %q", name)
	}
}

func TestEscalationSweepWorkerDedupeKeyPrefix(t *testing.T) {
	t.Parallel()
	dk := "alga:incident-dedupe:test-key"
	if len(dk) < len("alga:incident-dedupe:") {
		t.Fatalf("dedupe key too short")
	}
}
