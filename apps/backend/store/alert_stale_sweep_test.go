//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"alga/ent/alert"
	"alga/rabbitmq"
)

// backdateAlertByFingerprint moves an alert's created_at into the past so the
// stale sweep considers it old enough to investigate. The alert store forces
// created_at to now on Create, so this reaches into the ent client directly.
func backdateAlertByFingerprint(t *testing.T, fp string, age time.Duration) {
	t.Helper()
	ctx := context.Background()
	a, err := pgClient.Ent.Alert.Query().Where(alert.Fingerprint(fp)).First(ctx)
	if err != nil {
		t.Fatalf("find alert %q to backdate: %v", fp, err)
	}
	if _, err := pgClient.Ent.Alert.UpdateOneID(a.ID).
		SetCreatedAt(time.Now().UTC().Add(-age)).
		Save(ctx); err != nil {
		t.Fatalf("backdate alert %q: %v", fp, err)
	}
}

func TestListUninvestigatedAlertsSkipsAlertsLinkedToActiveIncident(t *testing.T) {
	ctx := context.Background()
	fp := "fp-stale-active-incident"

	if _, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "PostgreSQLDown", "instance": "pg2"},
	}); err != nil {
		t.Fatalf("create alert: %v", err)
	}
	backdateAlertByFingerprint(t, fp, 2*time.Hour)

	inc, err := incidentStore.CreateIncident(ctx, &IncidentRecord{
		Title:       "PostgreSQL pg2 offline",
		Status:      "active",
		Severity:    "critical",
		ImpactLevel: "medium",
		Priority:    "P2",
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	if err := alertsStore.LinkAlertToIncident(ctx, fp, inc.IncidentNumber); err != nil {
		t.Fatalf("link alert to incident: %v", err)
	}

	got, err := alertsStore.ListUninvestigatedAlerts(ctx, time.Hour)
	if err != nil {
		t.Fatalf("list uninvestigated: %v", err)
	}
	for _, a := range got {
		if a.Fingerprint == fp {
			t.Fatalf("active incident-linked alert returned as stale candidate")
		}
	}
}

func TestListUninvestigatedAlertsExcludesCascadeResolvedAlerts(t *testing.T) {
	ctx := context.Background()
	fp := "fp-stale-resolved-incident"

	if _, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "PostgreSQLDown", "instance": "pg3"},
	}); err != nil {
		t.Fatalf("create alert: %v", err)
	}
	backdateAlertByFingerprint(t, fp, 2*time.Hour)

	inc, err := incidentStore.CreateIncident(ctx, &IncidentRecord{
		Title:       "PostgreSQL pg3 resolved incident",
		Status:      "active",
		Severity:    "critical",
		ImpactLevel: "low",
		Priority:    "P3",
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	if err := alertsStore.LinkAlertToIncident(ctx, fp, inc.IncidentNumber); err != nil {
		t.Fatalf("link alert to incident: %v", err)
	}

	// Simulate the resolve-handler cascade: resolving the incident resolves its
	// linked firing alerts.
	res, err := alertsStore.ResolveAlertsByIncident(ctx, inc.IncidentNumber, &EventActor{
		Username: "tester", DisplayName: "Tester", Source: "incident_cascade",
	})
	if err != nil {
		t.Fatalf("ResolveAlertsByIncident: %v", err)
	}
	if len(res.Resolved) != 1 {
		t.Fatalf("cascade should have resolved the linked alert, got %#v", res)
	}

	got, err := alertsStore.ListUninvestigatedAlerts(ctx, time.Hour)
	if err != nil {
		t.Fatalf("list uninvestigated: %v", err)
	}
	for _, a := range got {
		if a.Fingerprint == fp {
			t.Fatalf("cascade-resolved alert must not appear as a stale candidate")
		}
	}
}

// TestListUninvestigatedAlertsReturnsAlertsSharingFingerprintWithPromotedInvestigation
// is the regression test for alert #90: a new firing alert that shares a
// fingerprint with a previously promoted investigation must still be returned
// as a stale candidate. Before the fix, "promoted" was missing from the
// terminal-status list, so the promoted investigation was treated as active
// forever and shadowed every subsequent firing alert with the same fingerprint.
func TestListUninvestigatedAlertsReturnsAlertsSharingFingerprintWithPromotedInvestigation(t *testing.T) {
	ctx := context.Background()
	fp := "fp-stale-promoted-investigation"

	// Create the alert first so the join row can resolve a real alert_id.
	if _, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "PostgreSQLDown", "instance": "pg-promoted"},
	}); err != nil {
		t.Fatalf("create alert: %v", err)
	}
	backdateAlertByFingerprint(t, fp, 2*time.Hour)

	inv, err := alertInvStore.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-promoted-shadow",
		CorrelationKey:       "pg-promoted",
		Alerts: []rabbitmq.CorrelatedAlert{
			{Fingerprint: fp, AlertNumber: 0, Labels: map[string]string{"alertname": "PostgreSQLDown"}},
		},
	})
	if err != nil {
		t.Fatalf("create alert investigation: %v", err)
	}
	if err := alertInvStore.UpdateAlertInvestigationStatus(ctx, inv.AlertInvestigationID, AlertInvestigationStatusPromoted); err != nil {
		t.Fatalf("mark investigation promoted: %v", err)
	}

	got, err := alertsStore.ListUninvestigatedAlerts(ctx, time.Hour)
	if err != nil {
		t.Fatalf("list uninvestigated: %v", err)
	}
	var found bool
	for _, a := range got {
		if a.Fingerprint == fp {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("firing alert shadowed by a promoted investigation must still be a stale candidate")
	}
}
