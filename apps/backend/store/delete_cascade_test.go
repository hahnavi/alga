package store

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"alga/ent/agentmemory"
	"alga/ent/alert"
	"alga/ent/alertinvestigationalert"
	"alga/rabbitmq"
)

func TestDeleteAlertHardDeletesLinkedInvestigationArtifacts(t *testing.T) {
	client := newTestEntClient(t)
	ctx := context.Background()
	alerts := newPGAlertStore(client)
	invs := newPGAlertInvestigationStore(client)
	threads := newPGInvestigationThreadStore(client)

	fp := "fp-cascade-alert"
	alertNumber, err := alerts.Create(AlertRecord{
		Fingerprint: fp, Status: "firing",
		Labels: map[string]string{"alertname": "CascadeAlert"},
	})
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}

	created, err := invs.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-CASCADE",
		Status:               AlertInvestigationStatusInvestigating,
		AgentID:              "agent-1",
		Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: fp, AlertNumber: alertNumber, Labels: map[string]string{"alertname": "CascadeAlert"}}},
	})
	if err != nil {
		t.Fatalf("create investigation: %v", err)
	}

	if _, err := threads.EnsureThread(ctx, ThreadOwnerAlert, strconv.FormatInt(alertNumber, 10)); err != nil {
		t.Fatalf("ensure thread: %v", err)
	}
	if _, err := client.AgentMemory.Create().
		SetInvestigationID(created.AlertInvestigationID).
		SetContent("rc").
		SetMemoryType("fact").
		SetHash("h-cascade").
		Save(ctx); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	if err := alerts.DeleteAlert(fp); err != nil {
		t.Fatalf("delete alert: %v", err)
	}

	// Parent kept as tombstone.
	got, err := alerts.GetByFingerprint(fp)
	if err != nil || got == nil || got.DeletedAt == nil {
		t.Fatalf("alert should remain as tombstone, got=%v err=%v", got, err)
	}
	// Investigation + children gone.
	if existing, _ := invs.GetAlertInvestigation(ctx, created.AlertInvestigationID); existing != nil {
		t.Fatal("investigation should be hard-deleted")
	}
	if n, _ := client.AlertInvestigationAlert.Query().
		Where(alertinvestigationalert.AlertInvestigationIDEQ(created.ID)).
		Count(ctx); n != 0 {
		t.Fatalf("expected 0 join rows, got %d", n)
	}
	if n, _ := client.AgentMemory.Query().Where(agentmemory.InvestigationIDEQ(created.AlertInvestigationID)).Count(ctx); n != 0 {
		t.Fatalf("expected 0 memories, got %d", n)
	}
	if _, _, err := threads.GetThreadByOwner(ctx, ThreadOwnerAlert, strconv.FormatInt(alertNumber, 10), 1, 0); !errorsIsNotFound(err) {
		t.Fatalf("thread should be gone, got err=%v", err)
	}
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrAlertNotFound)
}

// TestDeleteAlertDoesNotTouchSiblingInvestigationSharingFingerprint proves that
// hard-deleting one alert does not destroy an investigation belonging to a
// sibling alert that merely shares the same fingerprint. Fingerprints are dedup
// keys; alert_number is the unique identity, so the cascade must match by
// identity only.
func TestDeleteAlertDoesNotTouchSiblingInvestigationSharingFingerprint(t *testing.T) {
	client := newTestEntClient(t)
	ctx := context.Background()
	alerts := newPGAlertStore(client)
	invs := newPGAlertInvestigationStore(client)

	fp := "fp-shared-sibling"

	// a1: firing alert (included in the "one open alert per fingerprint"
	// partial unique index).
	a1Num, err := alerts.Create(AlertRecord{
		Fingerprint: fp, Status: "firing",
		Labels: map[string]string{"alertname": "Shared"},
	})
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}

	// a2: a SECOND alert row sharing the fingerprint but with a distinct
	// alert_number. Resolved alerts are excluded from the partial unique index
	// (status != 'resolved' AND deleted_at IS NULL), so a resolved + firing pair
	// can coexist — mirroring the real resolved/firing sibling coexistence.
	a2Num, err := alerts.Create(AlertRecord{
		Fingerprint: fp, Status: "resolved",
		Labels: map[string]string{"alertname": "Shared"},
	})
	if err != nil {
		t.Fatalf("create a2: %v", err)
	}
	if a2Num == a1Num {
		t.Fatalf("expected distinct alert numbers, got %d for both", a1Num)
	}

	inv1, err := invs.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-SIB-A",
		Status:               AlertInvestigationStatusInvestigating,
		Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: fp, AlertNumber: a1Num, Labels: map[string]string{"alertname": "Shared"}}},
	})
	if err != nil {
		t.Fatalf("create inv1: %v", err)
	}
	inv2, err := invs.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-SIB-B",
		Status:               AlertInvestigationStatusComplete,
		Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: fp, AlertNumber: a2Num, Labels: map[string]string{"alertname": "Shared"}}},
	})
	if err != nil {
		t.Fatalf("create inv2: %v", err)
	}

	// Delete a1 by its unique alert_number (precise identity).
	if err := alerts.DeleteAlertByNumber(a1Num); err != nil {
		t.Fatalf("delete a1: %v", err)
	}

	// a1's investigation must be hard-deleted.
	if got, _ := invs.GetAlertInvestigation(ctx, inv1.AlertInvestigationID); got != nil {
		t.Fatal("a1 investigation should be hard-deleted")
	}
	// a2's investigation MUST survive — matching by fingerprint would have
	// cross-contaminated this sibling investigation.
	if got, _ := invs.GetAlertInvestigation(ctx, inv2.AlertInvestigationID); got == nil {
		t.Fatal("sibling a2 investigation must NOT be deleted (fingerprint cross-contamination)")
	}
	// a2 alert itself must remain (not tombstoned) — only a1 was deleted.
	a2row, err := client.Alert.Query().Where(alert.AlertNumber(a2Num)).Only(ctx)
	if err != nil {
		t.Fatalf("query sibling a2: %v", err)
	}
	if a2row.DeletedAt != nil {
		t.Fatalf("sibling a2 alert must not be tombstoned, deleted_at=%v", a2row.DeletedAt)
	}
}

// TestDeleteAlertRemovesOwnerThreadEvenWithNoInvestigation covers the
// zero-investigation branch of hardDeleteAlertCascade: when an alert has an
// owner thread but no linked investigation, the thread + messages must still be
// removed when the alert is deleted.
func TestDeleteAlertRemovesOwnerThreadEvenWithNoInvestigation(t *testing.T) {
	client := newTestEntClient(t)
	ctx := context.Background()
	alerts := newPGAlertStore(client)
	threads := newPGInvestigationThreadStore(client)

	num, err := alerts.Create(AlertRecord{
		Fingerprint: "fp-thread-only", Status: "firing",
		Labels: map[string]string{"alertname": "ThreadOnly"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := threads.EnsureThread(ctx, ThreadOwnerAlert, strconv.FormatInt(num, 10)); err != nil {
		t.Fatalf("ensure thread: %v", err)
	}
	if err := alerts.DeleteAlertByNumber(num); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := threads.GetThreadByOwner(ctx, ThreadOwnerAlert, strconv.FormatInt(num, 10), 1, 0); !errorsIsNotFound(err) {
		t.Fatalf("thread should be gone, got err=%v", err)
	}
}

func TestDeleteIncidentHardDeletesIncidentInvestigationArtifacts(t *testing.T) {
	client := newTestEntClient(t)
	ctx := context.Background()
	incidents := newPGIncidentStore(client)
	incInvs := newPGIncidentInvestigationStore(client)
	threads := newPGInvestigationThreadStore(client)

	inc, err := incidents.CreateIncident(ctx, &IncidentRecord{
		Title: "Cascade Incident", Status: "active", Severity: "high", ImpactLevel: "low", Priority: "P3",
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	created, err := incInvs.CreateIncidentInvestigation(ctx, IncidentInvestigationRecord{
		IncidentInvestigationID: "IINV-CASCADE",
		IncidentNumber:          inc.IncidentNumber,
		Status:                  IncidentInvestigationStatusInvestigating,
		AgentID:                 "agent-2",
	})
	if err != nil {
		t.Fatalf("create incident investigation: %v", err)
	}
	if _, err := threads.EnsureThread(ctx, ThreadOwnerIncidentInvestigation, strconv.FormatInt(inc.IncidentNumber, 10)); err != nil {
		t.Fatalf("ensure thread: %v", err)
	}

	if err := incidents.DeleteIncident(ctx, inc.IncidentNumber); err != nil {
		t.Fatalf("delete incident: %v", err)
	}

	got, err := incidents.GetIncident(ctx, inc.IncidentNumber)
	if err != nil || got == nil || got.DeletedAt == nil {
		t.Fatalf("incident should remain as tombstone, got=%v err=%v", got, err)
	}
	if existing, _ := incInvs.GetIncidentInvestigation(ctx, created.IncidentInvestigationID); existing != nil {
		t.Fatal("incident investigation should be hard-deleted")
	}
	if _, _, err := threads.GetThreadByOwner(ctx, ThreadOwnerIncidentInvestigation, strconv.FormatInt(inc.IncidentNumber, 10), 1, 0); !errorsIsNotFound(err) {
		t.Fatalf("thread should be gone, got err=%v", err)
	}
}

func TestExpungeSoftDeletedAlertsChildrenIsIdempotent(t *testing.T) {
	client := newTestEntClient(t)
	ctx := context.Background()
	alerts := newPGAlertStore(client)
	invs := newPGAlertInvestigationStore(client)

	fp := "fp-expunge"
	num, err := alerts.Create(AlertRecord{Fingerprint: fp, Status: "firing", Labels: map[string]string{"alertname": "Expunge"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created, err := invs.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		AlertInvestigationID: "AINV-EXPUNGE", Status: AlertInvestigationStatusComplete,
		Alerts: []rabbitmq.CorrelatedAlert{{Fingerprint: fp, AlertNumber: num}},
	})
	if err != nil {
		t.Fatalf("create inv: %v", err)
	}
	if _, err := client.AgentMemory.Create().SetInvestigationID(created.AlertInvestigationID).SetContent("x").SetMemoryType("fact").SetHash("h-e").Save(ctx); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	// Simulate a pre-cascade tombstone: soft-delete the alert directly, bypassing
	// DeleteAlert (which now cascades) so the investigation + memory linger as
	// stale children — exactly the state the backfill targets.
	aRow, err := client.Alert.Query().Where(alert.Fingerprint(fp)).Only(ctx)
	if err != nil {
		t.Fatalf("lookup alert: %v", err)
	}
	if err := client.Alert.UpdateOneID(aRow.ID).SetDeletedAt(time.Now().UTC()).Exec(ctx); err != nil {
		t.Fatalf("soft-delete alert directly: %v", err)
	}

	expunger, ok := alerts.(alertExpungerTest)
	if !ok {
		t.Fatalf("alert store %T does not expose expunge", alerts)
	}
	n1, err := expunger.ExpungeSoftDeletedAlertsChildren(ctx)
	if err != nil {
		t.Fatalf("first expunge: %v", err)
	}
	if n1 < 1 {
		t.Fatalf("expected at least 1 parent processed, got %d", n1)
	}
	if got, _ := invs.GetAlertInvestigation(ctx, created.AlertInvestigationID); got != nil {
		t.Fatal("investigation should be expunged")
	}
	if c, _ := client.AgentMemory.Query().Where(agentmemory.InvestigationIDEQ(created.AlertInvestigationID)).Count(ctx); c != 0 {
		t.Fatalf("memory should be expunged, got %d", c)
	}
	// Idempotent: second run is a no-op, no error.
	if _, err := expunger.ExpungeSoftDeletedAlertsChildren(ctx); err != nil {
		t.Fatalf("second expunge should be a no-op, got: %v", err)
	}
}

// alertExpungerTest mirrors the main package's expunger interface so this test
// can reach the concrete method without adding it to the Store interface.
type alertExpungerTest interface {
	ExpungeSoftDeletedAlertsChildren(ctx context.Context) (int, error)
}
