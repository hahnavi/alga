//go:build integration

package store

import (
	"context"
	"testing"

	"alga/ent/alert"
	"alga/ent/alertevent"
)

func TestResolveAlertsByIncidentResolvesFiringAndSkipsResolved(t *testing.T) {
	ctx := context.Background()

	inc, err := incidentStore.CreateIncident(ctx, &IncidentRecord{
		Title: "cascade test", Status: "active", Severity: "high", ImpactLevel: "low", Priority: "P3",
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}

	fpFiring := "fp-cascade-firing"
	if _, err := alertsStore.Create(AlertRecord{
		Fingerprint: fpFiring, Status: "firing",
		Labels: map[string]string{"alertname": "FiringOne"},
	}); err != nil {
		t.Fatalf("create firing alert: %v", err)
	}
	if err := alertsStore.LinkAlertToIncident(ctx, fpFiring, inc.IncidentNumber); err != nil {
		t.Fatalf("link firing alert: %v", err)
	}

	fpResolved := "fp-cascade-already-resolved"
	if _, err := alertsStore.Create(AlertRecord{
		Fingerprint: fpResolved, Status: "resolved",
		Labels: map[string]string{"alertname": "AlreadyResolved"},
	}); err != nil {
		t.Fatalf("create resolved alert: %v", err)
	}
	if err := alertsStore.LinkAlertToIncident(ctx, fpResolved, inc.IncidentNumber); err != nil {
		t.Fatalf("link resolved alert: %v", err)
	}

	res, err := alertsStore.ResolveAlertsByIncident(ctx, inc.IncidentNumber, &EventActor{
		Username: "tester", DisplayName: "Tester", Source: "incident_cascade",
	})
	if err != nil {
		t.Fatalf("ResolveAlertsByIncident: %v", err)
	}

	if len(res.Resolved) != 1 || res.Resolved[0].Fingerprint != fpFiring {
		t.Fatalf("expected 1 resolved (%s), got %#v", fpFiring, res.Resolved)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Fingerprint != fpResolved {
		t.Fatalf("expected 1 skipped (%s), got %#v", fpResolved, res.Skipped)
	}
	if len(res.Failed) != 0 {
		t.Fatalf("expected 0 failed, got %#v", res.Failed)
	}

	a, err := pgClient.Ent.Alert.Query().Where(alert.Fingerprint(fpFiring)).Only(ctx)
	if err != nil {
		t.Fatalf("query firing alert: %v", err)
	}
	if a.Status != "resolved" {
		t.Fatalf("firing alert status = %q, want resolved", a.Status)
	}

	events, err := pgClient.Ent.AlertEvent.Query().
		Where(alertevent.HasAlertWith(alert.Fingerprint(fpFiring))).
		All(ctx)
	if err != nil {
		t.Fatalf("query alert events: %v", err)
	}
	var foundCascadeResolved bool
	for _, ev := range events {
		if ev.Type == "resolved" && ev.Source == "incident_cascade" {
			foundCascadeResolved = true
			break
		}
	}
	if !foundCascadeResolved {
		t.Fatalf("expected a resolved/incident_cascade alert event for %s, got %+v", fpFiring, events)
	}
}

func TestResolveAlertsByIncidentNoLinkedAlerts(t *testing.T) {
	ctx := context.Background()
	inc, err := incidentStore.CreateIncident(ctx, &IncidentRecord{
		Title: "empty cascade", Status: "active", Severity: "low", ImpactLevel: "low", Priority: "P4",
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	res, err := alertsStore.ResolveAlertsByIncident(ctx, inc.IncidentNumber, nil)
	if err != nil {
		t.Fatalf("ResolveAlertsByIncident: %v", err)
	}
	if len(res.Resolved) != 0 || len(res.Skipped) != 0 || len(res.Failed) != 0 {
		t.Fatalf("expected empty result, got %#v", res)
	}
}
