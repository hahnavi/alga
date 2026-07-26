package api

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"alga/sse"
	"alga/store"
)

type cascadeFakeAlertStore struct {
	result store.AlertCascadeResult
	err    error
	actor  *store.EventActor
}

func (f *cascadeFakeAlertStore) ResolveAlertsByIncident(_ context.Context, _ int64, actor *store.EventActor) (store.AlertCascadeResult, error) {
	f.actor = actor
	return f.result, f.err
}

type cascadeFakeAudit struct {
	count  atomic.Int32
	last   store.AuditEvent
	lastIP string
	lastUA string
}

func (a *cascadeFakeAudit) Log(event store.AuditEvent, _ *uuid.UUID, _ string, ip string, ua string, _ bool, _ map[string]any) {
	a.count.Add(1)
	a.last = event
	a.lastIP = ip
	a.lastUA = ua
}
func (a *cascadeFakeAudit) LogEntity(event store.AuditEvent, _ *uuid.UUID, _ string, ip string, ua string, _ bool, _ map[string]any, _ string, _ *uuid.UUID) {
	a.count.Add(1)
	a.last = event
	a.lastIP = ip
	a.lastUA = ua
}
func (a *cascadeFakeAudit) Query(filter map[string]any) ([]store.AuditRecord, error) { return nil, nil }
func (a *cascadeFakeAudit) GetRecentEvents(limit int) ([]store.AuditRecord, error)   { return nil, nil }

type cascadeFakeSSE struct {
	events []sse.Event
}

func (p *cascadeFakeSSE) Publish(event sse.Event) {
	p.events = append(p.events, event)
}

func TestRunAlertCascadeEmitsAuditAndSSEPerResolvedAlert(t *testing.T) {
	alerts := &cascadeFakeAlertStore{result: store.AlertCascadeResult{
		Resolved: []store.AlertRecord{
			{AlertNumber: 1, Fingerprint: "fp-1", Status: "resolved"},
			{AlertNumber: 2, Fingerprint: "fp-2", Status: "resolved"},
		},
		Skipped: []store.AlertRef{{AlertNumber: 3, Fingerprint: "fp-3"}},
	}}
	audit := &cascadeFakeAudit{}
	pub := &cascadeFakeSSE{}

	res := runAlertCascade(context.Background(), alerts, audit, pub, 77, CascadeActor{
		ID:        uuid.New(),
		Type:      "user",
		Name:      "ops@example.com",
		IP:        "10.0.0.1:5566",
		UserAgent: "CascadeTestAgent/1.0",
	})

	if len(res.Resolved) != 2 {
		t.Fatalf("resolved = %d, want 2", len(res.Resolved))
	}
	if audit.count.Load() != 2 {
		t.Fatalf("audit count = %d, want 2 (one per resolved, none for skipped)", audit.count.Load())
	}
	if audit.last != store.AuditAlertAutoResolved {
		t.Fatalf("audit event = %q, want %q", audit.last, store.AuditAlertAutoResolved)
	}
	if audit.lastIP != "10.0.0.1:5566" {
		t.Fatalf("audit ip = %q, want actor IP propagated to Log", audit.lastIP)
	}
	if audit.lastUA != "CascadeTestAgent/1.0" {
		t.Fatalf("audit user-agent = %q, want actor UA propagated to Log", audit.lastUA)
	}
	if len(pub.events) != 2 {
		t.Fatalf("sse events = %d, want 2 alert_updated", len(pub.events))
	}
	for _, ev := range pub.events {
		if ev.Type != "alert_updated" {
			t.Fatalf("sse event type = %q, want alert_updated", ev.Type)
		}
		rec, ok := ev.Data.(store.AlertRecord)
		if !ok {
			t.Fatalf("sse event data type = %T, want store.AlertRecord (full record for frontend)", ev.Data)
		}
		if rec.Status != "resolved" {
			t.Fatalf("broadcast record status = %q, want resolved", rec.Status)
		}
		if rec.AlertNumber == 0 || rec.Fingerprint == "" {
			t.Fatalf("broadcast record missing identity: %#v", rec)
		}
	}
	if alerts.actor == nil || alerts.actor.Source != "incident_cascade" {
		t.Fatalf("actor source = %v, want incident_cascade", alerts.actor)
	}
}

func TestRunAlertCascadePublishesPartialEventOnFailure(t *testing.T) {
	alerts := &cascadeFakeAlertStore{result: store.AlertCascadeResult{
		Resolved: []store.AlertRecord{{AlertNumber: 1, Fingerprint: "fp-1", Status: "resolved"}},
		Failed:   []store.AlertRef{{AlertNumber: 2, Fingerprint: "fp-2"}},
	}}
	pub := &cascadeFakeSSE{}

	runAlertCascade(context.Background(), alerts, &cascadeFakeAudit{}, pub, 1, CascadeActor{ID: uuid.New(), Type: "agent", Name: "responder-1"})

	var partial *sse.Event
	for i := range pub.events {
		if pub.events[i].Type == "incident_alert_cascade_partial" {
			partial = &pub.events[i]
		}
	}
	if partial == nil {
		t.Fatalf("expected incident_alert_cascade_partial event when failed > 0, got %#v", pub.events)
	}
}

func TestRunAlertCascadeNilDepsAreNoOp(t *testing.T) {
	alerts := &cascadeFakeAlertStore{result: store.AlertCascadeResult{
		Resolved: []store.AlertRecord{{AlertNumber: 1, Fingerprint: "fp-1", Status: "resolved"}},
	}}
	res := runAlertCascade(context.Background(), alerts, nil, nil, 1, CascadeActor{ID: uuid.New(), Type: "user", Name: "x"})
	if len(res.Resolved) != 1 {
		t.Fatalf("expected result propagated with nil audit/sse, got %#v", res)
	}
}
