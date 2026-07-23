//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestDeleteAlert_SoftDeletesPreservesRow verifies that deleting an alert sets
// deleted_at but keeps the row (and its events/delivery targets) queryable, and
// that GetOpenByFingerprint no longer returns it.
func TestDeleteAlert_SoftDeletesPreservesRow(t *testing.T) {
	fp := "softdel-alert-" + t.Name()

	created, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "SoftDelTest"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created == 0 {
		t.Fatal("expected alert number")
	}

	if err := alertsStore.DeleteAlert(fp); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	// Dedup lookup must NOT find the soft-deleted alert.
	got, err := alertsStore.GetOpenByFingerprint(fp)
	if err != nil {
		t.Fatalf("GetOpenByFingerprint: %v", err)
	}
	if got != nil {
		t.Fatalf("GetOpenByFingerprint returned soft-deleted alert: %+v", got)
	}

	// By-fingerprint tombstone lookup MUST still find it, with deleted_at set.
	rec, err := alertsStore.GetByFingerprint(fp)
	if err != nil {
		t.Fatalf("GetByFingerprint: %v", err)
	}
	if rec == nil {
		t.Fatal("expected soft-deleted alert to still exist for tombstone")
	}
	if rec.DeletedAt == nil {
		t.Error("DeletedAt not set after DeleteAlert")
	}
}

// TestDeleteAlert_Idempotent verifies a second DeleteAlert on the same
// fingerprint returns ErrAlertNotFound (the lookup's DeletedAtIsNil filter
// suppresses soft-deleted rows).
func TestDeleteAlert_Idempotent(t *testing.T) {
	fp := "softdel-idempotent-" + t.Name()
	_, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "SoftDelIdempotent"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := alertsStore.DeleteAlert(fp); err != nil {
		t.Fatalf("first DeleteAlert: %v", err)
	}

	err = alertsStore.DeleteAlert(fp)
	if err == nil {
		t.Fatal("second DeleteAlert: expected ErrAlertNotFound, got nil")
	}
	if !errors.Is(err, ErrAlertNotFound) {
		t.Fatalf("second DeleteAlert: expected ErrAlertNotFound, got %v", err)
	}
}

// TestDeleteAlertByNumber_SoftDeletes verifies the by-number path also
// soft-deletes (sets DeletedAt) rather than hard-deleting.
func TestDeleteAlertByNumber_SoftDeletes(t *testing.T) {
	fp := "softdel-bynum-" + t.Name()
	n, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "SoftDelByNum"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := alertsStore.DeleteAlertByNumber(n); err != nil {
		t.Fatalf("DeleteAlertByNumber: %v", err)
	}

	rec, err := alertsStore.GetByFingerprint(fp)
	if err != nil {
		t.Fatalf("GetByFingerprint: %v", err)
	}
	if rec == nil || rec.DeletedAt == nil {
		t.Fatal("expected soft-deleted alert with DeletedAt set after DeleteAlertByNumber")
	}
}

// TestSoftDeletedAlert_ExcludedFromReads verifies a soft-deleted alert is
// excluded from list/dedup reads but reachable via tombstone lookups.
func TestSoftDeletedAlert_ExcludedFromReads(t *testing.T) {
	fp := "softdel-reads-" + t.Name()

	_, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "SoftDelReadsTest"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := alertsStore.DeleteAlert(fp); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	if got, _ := alertsStore.GetOpenByFingerprint(fp); got != nil {
		t.Errorf("GetOpenByFingerprint should not return soft-deleted alert")
	}

	res, err := alertsStore.QueryAlerts(map[string]any{"fingerprint": fp})
	if err != nil {
		t.Fatalf("QueryAlerts: %v", err)
	}
	for _, r := range res {
		if r.Fingerprint == fp && r.DeletedAt == nil {
			t.Errorf("QueryAlerts returned non-deleted alert for soft-deleted fingerprint")
		}
	}
}

// TestAlertDedup_DeletedAllowsNewFiring verifies that a soft-deleted firing
// alert does not suppress a brand-new firing alert for the same fingerprint,
// proving the partial unique index predicate
// (status != 'resolved' AND deleted_at IS NULL) is honored end-to-end.
func TestAlertDedup_DeletedAllowsNewFiring(t *testing.T) {
	fp := "deleted-then-firing-" + t.Name()

	first, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "DedupDelTest", "severity": "warning"},
	})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	if err := alertsStore.DeleteAlert(fp); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	// A new firing webhook for the same fingerprint must create a fresh alert.
	second, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "DedupDelTest", "severity": "critical"},
	})
	if err != nil {
		t.Fatalf("second Create (should succeed): %v", err)
	}
	if second == first {
		t.Fatal("expected a brand-new alert number, got the soft-deleted one")
	}

	// Dedup lookup returns the fresh alert, not the deleted one.
	got, err := alertsStore.GetOpenByFingerprint(fp)
	if err != nil {
		t.Fatalf("GetOpenByFingerprint: %v", err)
	}
	if got.DeletedAt != nil {
		t.Errorf("GetOpenByFingerprint returned a soft-deleted alert")
	}
	if got.Labels["severity"] != "critical" {
		t.Errorf("Severity = %q, want critical (fresh alert)", got.Labels["severity"])
	}
}

// TestGetAlertsByIncident_IncludesSoftDeletedTombstones asserts the
// incident→alerts read returns soft-deleted alerts so the handler can render
// them as tombstones in the Linked Alerts card. Spec: "include deleted" rule.
func TestGetAlertsByIncident_IncludesSoftDeletedTombstones(t *testing.T) {
	fp := "incident-link-tombstone-" + t.Name()

	created, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "LinkedTombstone"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ctx := context.Background()
	inc, err := incidentStore.CreateIncident(ctx, &IncidentRecord{
		Title:       "LinkTombstoneIncident",
		Status:      "detected",
		Severity:    "warning",
		ImpactLevel: "low",
		Priority:    "P5",
	})
	if err != nil {
		t.Fatalf("CreateIncident: %v", err)
	}

	if err := alertsStore.LinkAlertToIncident(ctx, fp, inc.IncidentNumber); err != nil {
		t.Fatalf("LinkAlertToIncident: %v", err)
	}
	if err := alertsStore.DeleteAlert(fp); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	fingerprints, err := alertsStore.GetAlertsByIncident(ctx, inc.IncidentNumber)
	if err != nil {
		t.Fatalf("GetAlertsByIncident: %v", err)
	}

	var found *AlertRecord
	for _, resultFP := range fingerprints {
		if resultFP != fp {
			continue
		}
		rec, err := alertsStore.GetByFingerprint(resultFP)
		if err != nil {
			t.Fatalf("GetByFingerprint(%q): %v", resultFP, err)
		}
		if rec == nil {
			t.Fatalf("GetByFingerprint(%q) returned nil record", resultFP)
		}
		if rec.AlertNumber == created {
			found = rec
			break
		}
	}
	if found == nil {
		t.Fatal("GetAlertsByIncident should include the soft-deleted alert's fingerprint")
	}
	if found.DeletedAt == nil {
		t.Fatal("GetByFingerprint should return the soft-deleted alert with DeletedAt set (tombstone)")
	}
	if found.Fingerprint != fp {
		t.Fatalf("tombstone fingerprint = %q, want %q", found.Fingerprint, fp)
	}
}

// TestDeleteOlderThan_PreservesSoftDeletedAlerts ensures the retention sweep
// does NOT hard-purge soft-deleted rows. Soft-deleted alerts must survive
// until a separate (out-of-scope) purge job lands.
func TestDeleteOlderThan_PreservesSoftDeletedAlerts(t *testing.T) {
	fp := "retention-softdel-" + t.Name()
	ctx := context.Background()
	oldTime := time.Now().Add(-365 * 24 * time.Hour) // ancient

	_, err := alertsStore.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "resolved",
		StartsAt:    oldTime,
		Labels:      map[string]string{"alertname": "RetentionSoftDel"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := alertsStore.DeleteAlert(fp); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}

	deleted, err := alertsStore.DeleteOlderThan(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}
	if deleted != 0 {
		t.Errorf("DeleteOlderThan hard-purged %d soft-deleted alert(s); soft-deleted rows must survive retention sweeps", deleted)
	}

	// Tombstone must still be queryable.
	rec, err := alertsStore.GetByFingerprint(fp)
	if err != nil || rec == nil {
		t.Fatalf("GetByFingerprint: %v", err)
	}
	if rec.DeletedAt == nil {
		t.Error("soft-deleted alert was hard-purged by retention sweep")
	}
}
