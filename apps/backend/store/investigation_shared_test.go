package store

import "testing"

func TestIsTerminalInvestigationStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   bool
	}{
		{"complete", true},
		{"failed", true},
		{"cancelled", true},
		{"timed_out", true},
		{"promoted", true},
		{"pending", false},
		{"assigned", false},
		{"investigating", false},
		{"paused", false},
		{"", false},
		{"unknown", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := IsTerminalInvestigationStatus(tc.status); got != tc.want {
				t.Fatalf("IsTerminalInvestigationStatus(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestPromotedIsTerminalNotReopenable pins the regression fix for alert #90:
// a promoted investigation is terminal (so the correlator and stale-alert sweep
// stop treating it as an active investigation that shadows future firing
// alerts with the same fingerprint), but it must NOT be reopenable, since
// reopening it would silently demote it back into active duty while the
// promoted-to incident still owns it.
func TestPromotedIsTerminalNotReopenable(t *testing.T) {
	t.Parallel()
	if !IsTerminalInvestigationStatus(AlertInvestigationStatusPromoted) {
		t.Fatalf("promoted must be a terminal status")
	}
	if IsReopenableInvestigationStatus(AlertInvestigationStatusPromoted) {
		t.Fatalf("promoted must not be reopenable; reopening would orphan the promoted-to incident")
	}
}

func TestIsReopenableInvestigationStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   bool
	}{
		{"complete", true},
		{"failed", true},
		{"cancelled", true},
		{"timed_out", true},
		{"paused", true},
		{"promoted", false},
		{"pending", false},
		{"assigned", false},
		{"investigating", false},
		{"", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := IsReopenableInvestigationStatus(tc.status); got != tc.want {
				t.Fatalf("IsReopenableInvestigationStatus(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}
