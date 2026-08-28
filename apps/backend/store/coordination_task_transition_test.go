package store

import (
	"slices"
	"testing"
)

// TestCanTransitionCoordinationTask covers the authoritative coordination-task
// state machine: every legal edge, the accepted same-status no-op, and the
// illegal jumps (most importantly pending → complete, which must never succeed
// even though both endpoints are individually valid states).
func TestCanTransitionCoordinationTask(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		// Legal edges.
		{"pending to assigned", CoordinationTaskStatusPending, CoordinationTaskStatusAssigned, false},
		{"pending to failed", CoordinationTaskStatusPending, CoordinationTaskStatusFailed, false},
		{"pending to cancelled", CoordinationTaskStatusPending, CoordinationTaskStatusCancelled, false},
		{"assigned to in_progress", CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress, false},
		{"assigned back to pending", CoordinationTaskStatusAssigned, CoordinationTaskStatusPending, false},
		{"assigned to failed", CoordinationTaskStatusAssigned, CoordinationTaskStatusFailed, false},
		{"assigned to cancelled", CoordinationTaskStatusAssigned, CoordinationTaskStatusCancelled, false},
		{"in_progress to complete", CoordinationTaskStatusInProgress, CoordinationTaskStatusComplete, false},
		{"in_progress to failed", CoordinationTaskStatusInProgress, CoordinationTaskStatusFailed, false},
		{"in_progress reverted to pending", CoordinationTaskStatusInProgress, CoordinationTaskStatusPending, false},
		{"in_progress to cancelled", CoordinationTaskStatusInProgress, CoordinationTaskStatusCancelled, false},

		// Same-status restatement is the accepted no-op.
		{"pending restated", CoordinationTaskStatusPending, CoordinationTaskStatusPending, false},
		{"in_progress restated", CoordinationTaskStatusInProgress, CoordinationTaskStatusInProgress, false},

		// Illegal jumps and terminal sinks.
		{"pending to complete rejected", CoordinationTaskStatusPending, CoordinationTaskStatusComplete, true},
		{"pending to in_progress rejected", CoordinationTaskStatusPending, CoordinationTaskStatusInProgress, true},
		{"assigned to complete rejected", CoordinationTaskStatusAssigned, CoordinationTaskStatusComplete, true},
		{"complete is terminal", CoordinationTaskStatusComplete, CoordinationTaskStatusPending, true},
		{"failed is terminal", CoordinationTaskStatusFailed, CoordinationTaskStatusPending, true},
		{"cancelled is terminal", CoordinationTaskStatusCancelled, CoordinationTaskStatusPending, true},
		{"unknown from status", "bogus", CoordinationTaskStatusPending, true},
		{"unknown to status", CoordinationTaskStatusPending, "bogus", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanTransitionCoordinationTask(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CanTransitionCoordinationTask(%q, %q) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

// TestCoordinationTaskSourcesTo verifies the derived CAS source sets used by
// FailTask/CancelTask/RevertByAgent/BumpDispatchAttempts: derived purely from
// the map, deterministically sorted, self-edges excluded.
func TestCoordinationTaskSourcesTo(t *testing.T) {
	expect := func(got []string, want ...string) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	expect(coordinationTaskSourcesTo(CoordinationTaskStatusFailed),
		CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress, CoordinationTaskStatusPending)
	expect(coordinationTaskSourcesTo(CoordinationTaskStatusCancelled),
		CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress, CoordinationTaskStatusPending)
	expect(coordinationTaskSourcesTo(CoordinationTaskStatusComplete), CoordinationTaskStatusInProgress)
	expect(coordinationTaskSourcesTo(CoordinationTaskStatusPending),
		CoordinationTaskStatusAssigned, CoordinationTaskStatusInProgress)
	expect(coordinationTaskSourcesTo("bogus"))
}
