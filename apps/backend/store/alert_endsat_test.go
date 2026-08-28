//go:build integration

package store

import (
	"testing"
	"time"
)

// TestAlertEndsAtOnResolution verifies the ends_at contract for resolved
// alerts: resolution stamps ends_at when the ingest payload did not supply
// one, and preserves an ingest-supplied value.
func TestAlertEndsAt(t *testing.T) {
	stores := newTestStores(t)

	// Case 1: alert ingested without endsAt gets ends_at stamped on resolve.
	fpNoEnd := "endsat-stamp-" + t.Name()
	if _, err := stores.Alert.Create(AlertRecord{
		Fingerprint: fpNoEnd,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "EndsAtStamp", "severity": "warning"},
		StartsAt:    time.Now().UTC().Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("create alert without endsAt: %v", err)
	}

	if err := stores.Alert.UpdateStatus(fpNoEnd, "resolved", nil); err != nil {
		t.Fatalf("resolve alert without endsAt: %v", err)
	}

	got, err := stores.Alert.GetByFingerprint(fpNoEnd)
	if err != nil {
		t.Fatalf("get alert after resolve: %v", err)
	}
	if got == nil || got.Status != "resolved" {
		t.Fatalf("expected resolved alert, got %+v", got)
	}
	if got.EndsAt == nil {
		t.Fatal("ends_at not populated on monitor-driven resolution")
	}

	t.Cleanup(func() { _ = stores.Alert.DeleteAlertByNumber(got.AlertNumber) })

	// Case 2: an explicit ingest-time endsAt survives resolution unchanged.
	fpWithEnd := "endsat-keep-" + t.Name()
	// Postgres timestamptz keeps only microsecond precision; pre-truncate so
	// the round-trip comparison is exact.
	ingestEnd := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	alertNumber, err := stores.Alert.Create(AlertRecord{
		Fingerprint: fpWithEnd,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "EndsAtKeep", "severity": "warning"},
		StartsAt:    time.Now().UTC().Add(-10 * time.Minute),
		EndsAt:      &ingestEnd,
	})
	if err != nil {
		t.Fatalf("create alert with endsAt: %v", err)
	}

	if err := stores.Alert.UpdateStatus(fpWithEnd, "resolved", nil); err != nil {
		t.Fatalf("resolve alert with endsAt: %v", err)
	}

	got, err = stores.Alert.GetByFingerprint(fpWithEnd)
	if err != nil {
		t.Fatalf("get alert after resolve: %v", err)
	}
	if got == nil || got.Status != "resolved" {
		t.Fatalf("expected resolved alert, got %+v", got)
	}
	if got.EndsAt == nil {
		t.Fatal("ends_at lost during resolution")
	}
	// Postgres stores timestamptz (microsecond precision, returned in the
	// server's timezone); compare instants rather than wall-clock fields.
	if !got.EndsAt.Equal(ingestEnd) {
		t.Fatalf("ingest-supplied ends_at clobbered: got %v (%s), want %v",
			got.EndsAt.UTC(), got.EndsAt.Location(), ingestEnd)
	}

	t.Cleanup(func() { _ = stores.Alert.DeleteAlertByNumber(alertNumber) })
}

// TestAlertSilencedEndsAt verifies UpdateStatusSilenced also stamps ends_at
// when it was NULL at ingest.
func TestAlertSilencedEndsAt(t *testing.T) {
	stores := newTestStores(t)

	fp := "endsat-silence-" + t.Name()
	if _, err := stores.Alert.Create(AlertRecord{
		Fingerprint: fp,
		Status:      "firing",
		Labels:      map[string]string{"alertname": "EndsAtSilenced", "severity": "info"},
		StartsAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create alert: %v", err)
	}

	if err := stores.Alert.UpdateStatusSilenced(fp); err != nil {
		t.Fatalf("silence-resolve alert: %v", err)
	}

	got, err := stores.Alert.GetByFingerprint(fp)
	if err != nil {
		t.Fatalf("get silenced alert: %v", err)
	}
	if got == nil || got.Status != "resolved" || !got.Silenced {
		t.Fatalf("expected silenced+resolved alert, got %+v", got)
	}
	if got.EndsAt == nil {
		t.Fatal("ends_at not populated by UpdateStatusSilenced")
	}

	t.Cleanup(func() { _ = stores.Alert.DeleteAlertByNumber(got.AlertNumber) })
}
