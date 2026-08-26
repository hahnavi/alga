//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

// TestShiftAlertMetrics pins the per-shift counters contract wired into
// GET /api/v1/on-call/metrics: alerts firing inside the window count as
// received (and missed when never acknowledged by window end); ack/resolve
// events are counted by their event timestamps; avg ack time pairs each
// fired-in-window alert with its first-ever acknowledge.
func TestShiftAlertMetrics(t *testing.T) {
	stores := newTestStores(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Microsecond)
	winStart := base.Add(-time.Hour)
	winEnd := base.Add(time.Hour)

	// A1: fires inside the window and is acknowledged — received + acked, and
	// contributes to the avg-ack pairing.
	a1, err := stores.Alert.Create(AlertRecord{
		Fingerprint: "sam-acked-" + t.Name(),
		Status:      "firing",
		Labels:      map[string]string{"alertname": "SAM", "severity": "warning"},
		StartsAt:    base,
	})
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	t.Cleanup(func() { _ = stores.Alert.DeleteAlertByNumber(a1) })
	if err := stores.Alert.AcknowledgeAlertByNumber(a1, &EventActor{UserID: "u1", Username: "u1"}); err != nil {
		t.Fatalf("acknowledge a1: %v", err)
	}

	// A2: fires inside the window and is never acknowledged — missed.
	a2, err := stores.Alert.Create(AlertRecord{
		Fingerprint: "sam-missed-" + t.Name(),
		Status:      "firing",
		Labels:      map[string]string{"alertname": "SAM", "severity": "critical"},
		StartsAt:    base.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create a2: %v", err)
	}
	t.Cleanup(func() { _ = stores.Alert.DeleteAlertByNumber(a2) })

	// A3: fired before the window but acknowledged during it — counts toward
	// `acknowledged` only (documented window-based event attribution).
	a3, err := stores.Alert.Create(AlertRecord{
		Fingerprint: "sam-outside-" + t.Name(),
		Status:      "firing",
		Labels:      map[string]string{"alertname": "SAM", "severity": "info"},
		StartsAt:    base.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create a3: %v", err)
	}
	t.Cleanup(func() { _ = stores.Alert.DeleteAlertByNumber(a3) })
	if err := stores.Alert.AcknowledgeAlertByNumber(a3, nil); err != nil {
		t.Fatalf("acknowledge a3: %v", err)
	}

	m, err := stores.Alert.ShiftAlertMetrics(ctx, winStart, winEnd)
	if err != nil {
		t.Fatalf("ShiftAlertMetrics: %v", err)
	}
	if m.Received != 2 {
		t.Errorf("received = %d, want 2", m.Received)
	}
	if m.Acknowledged != 2 {
		t.Errorf("acknowledged = %d, want 2", m.Acknowledged)
	}
	if m.Resolved != 0 {
		t.Errorf("resolved = %d, want 0", m.Resolved)
	}
	if m.Missed != 1 {
		t.Errorf("missed = %d, want 1", m.Missed)
	}
	if m.AvgAckSeconds <= 0 || m.AvgAckSeconds > 3600 {
		t.Errorf("avg_ack_seconds = %v, want in (0, 3600]", m.AvgAckSeconds)
	}

	// Inverted windows are rejected rather than silently empty.
	if _, err := stores.Alert.ShiftAlertMetrics(ctx, winEnd, winStart); err == nil {
		t.Error("inverted window accepted; want error")
	}

	// Empty windows report all zeros without error.
	empty, err := stores.Alert.ShiftAlertMetrics(ctx, base.Add(-31*24*time.Hour), base.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("ShiftAlertMetrics(empty): %v", err)
	}
	if empty.Received != 0 || empty.Acknowledged != 0 || empty.Resolved != 0 || empty.Missed != 0 || empty.AvgAckSeconds != 0 {
		t.Errorf("expected zero metrics on an empty window, got %+v", empty)
	}
}
