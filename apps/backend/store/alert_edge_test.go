//go:build integration

package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"alga/rabbitmq"
)

func TestConcurrentAlertCreation_SameFingerprint(t *testing.T) {
	fp := "concurrent-fp-" + t.Name()

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		failures  int
	)

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := alertsStore.Create(AlertRecord{
				Fingerprint: fp,
				Status:      "firing",
				Labels:      map[string]string{"alertname": "ConcurrentTest"},
			})
			mu.Lock()
			if err != nil {
				failures++
			} else {
				successes++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 successful create, got %d (failures=%d)", successes, failures)
	}
	if failures != 9 {
		t.Errorf("expected exactly 9 failures, got %d (successes=%d)", failures, successes)
	}

	got, err := alertsStore.GetByFingerprint(fp)
	if err != nil {
		t.Fatalf("GetByFingerprint: %v", err)
	}
	if got == nil {
		t.Fatal("expected alert to exist")
	}
	if got.Status != "firing" {
		t.Errorf("Status = %q, want firing", got.Status)
	}

	t.Cleanup(func() { alertsStore.DeleteAlert(fp) })
}

func TestInvestigationStateMachine_InvalidTransition(t *testing.T) {
	ctx := context.Background()

	rec, err := alertInvStore.CreateAlertInvestigation(ctx, AlertInvestigationRecord{
		CorrelationKey: "transition-test",
		Status:         AlertInvestigationStatusPending,
		Alerts: []rabbitmq.CorrelatedAlert{
			{
				Fingerprint:  "fp-transition-test",
				Labels:       map[string]string{"alertname": "TransitionTest"},
				Status:       "firing",
				StartsAt:     time.Now().Format(time.RFC3339),
				GeneratorURL: "http://grafana/transition",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateAlertInvestigation: %v", err)
	}
	invID := rec.AlertInvestigationID

	if err := alertInvStore.UpdateAlertInvestigationStatus(ctx, invID, AlertInvestigationStatusInvestigating); err != nil {
		t.Fatalf("UpdateAlertInvestigationStatus investigating: %v", err)
	}

	if err := alertInvStore.UpdateAlertInvestigationStatus(ctx, invID, AlertInvestigationStatusComplete); err != nil {
		t.Fatalf("UpdateAlertInvestigationStatus complete: %v", err)
	}

	// TransitionAlertInvestigationStatus wraps the underlying database update
	// failure in a generic error (no status-conflict sentinel exists for
	// alert investigations after the investigation-domain split), so we
	// assert a non-nil error plus the meaningful conflict contract: the
	// status must not have changed (asserted below).
	err = alertInvStore.TransitionAlertInvestigationStatus(ctx, rec.ID.String(), []string{AlertInvestigationStatusPending}, AlertInvestigationStatusInvestigating)
	if err == nil {
		t.Fatal("expected error when transitioning from completed to investigating with wrong fromStatus")
	}

	got, _ := alertInvStore.GetAlertInvestigation(ctx, invID)
	if got.Status != AlertInvestigationStatusComplete {
		t.Errorf("Status = %q, want complete (should be unchanged)", got.Status)
	}

	t.Cleanup(func() { alertInvStore.DeleteAlertInvestigation(ctx, invID) })
}

func TestAlertDedup_ResolvedAllowsNewFiring(t *testing.T) {
	fp := "resolved-then-firing-" + t.Name()

	_, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "DedupTest", "severity": "warning"},
	})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	err = alertsStore.UpdateStatus(fp, "resolved", nil)
	if err != nil {
		t.Fatalf("UpdateStatus resolved: %v", err)
	}

	got, err := alertsStore.GetByFingerprint(fp)
	if err != nil {
		t.Fatalf("GetByFingerprint after resolve: %v", err)
	}
	if got.Status != "resolved" {
		t.Fatalf("Status = %q, want resolved before second create", got.Status)
	}

	_, err = alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "DedupTest", "severity": "critical"},
	})
	if err != nil {
		t.Fatalf("second Create (should succeed): %v", err)
	}

	got, err = alertsStore.GetByFingerprint(fp)
	if err != nil {
		t.Fatalf("GetByFingerprint after second create: %v", err)
	}
	if got == nil {
		t.Fatal("expected alert to exist after second create")
	}
	if got.Status != "firing" {
		t.Errorf("Status = %q, want firing", got.Status)
	}
	if got.Labels["severity"] != "critical" {
		t.Errorf("Severity = %q, want critical (new alert, not the resolved one)", got.Labels["severity"])
	}

	t.Cleanup(func() {
		alertsStore.UpdateStatus(fp, "resolved", nil)
		alertsStore.DeleteAlert(fp)
	})
}
