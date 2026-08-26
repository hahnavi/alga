//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/db"
	"alga/db/models"
)

// TestCoordinationTaskTransitionGuard exercises the store-level transition
// guard end-to-end on a migrated database: illegal jumps are rejected with
// ErrCoordinationTaskStatusConflict before touching SQL, terminal tasks reject
// fail/cancel, and the happy path pending → assigned → in_progress → complete
// still works.
func TestCoordinationTaskTransitionGuard(t *testing.T) {
	bunDB := newTestDB(t)
	cli := &db.Client{DB: bunDB}
	stores, err := NewStores(cli, time.Hour, 12*time.Hour)
	if err != nil {
		t.Fatalf("create stores: %v", err)
	}
	ctx := context.Background()
	ts := stores.CoordinationTask

	cleanup := func(id uuid.UUID) {
		t.Cleanup(func() {
			_, _ = bunDB.NewDelete().Model((*models.CoordinationTask)(nil)).Where("id = ?", id).Exec(context.Background())
		})
	}

	createPending := func(t *testing.T) *CoordinationTaskRecord {
		t.Helper()
		rec, err := ts.CreateTask(ctx, &CoordinationTaskRecord{
			Kind:         CoordinationTaskKindInvestigate,
			AssigneeRole: CoordinationTaskRoleResponder,
			Goal:         "guard test task",
		})
		if err != nil {
			t.Fatalf("create coordination task: %v", err)
		}
		cleanup(rec.ID)
		return rec
	}

	assertConflict := func(t *testing.T, err error, stage string) {
		t.Helper()
		if !errors.Is(err, ErrCoordinationTaskStatusConflict) {
			t.Fatalf("%s: expected ErrCoordinationTaskStatusConflict, got %v", stage, err)
		}
	}

	t.Run("pending to complete rejected via UpdateTaskStatus", func(t *testing.T) {
		task := createPending(t)
		err := ts.UpdateTaskStatus(ctx, task.ID, []string{CoordinationTaskStatusPending}, CoordinationTaskStatusComplete)
		assertConflict(t, err, "UpdateTaskStatus pending→complete")
	})

	t.Run("complete and fail and cancel rejected from pending where illegal", func(t *testing.T) {
		task := createPending(t)
		assertConflict(t, ts.CompleteTask(ctx, task.ID, map[string]any{"ok": true}), "CompleteTask from pending")

		// Cancelling a pending task is legal; repeating it or failing it after
		// the terminal transition must be rejected deterministically.
		if err := ts.CancelTask(ctx, task.ID); err != nil {
			t.Fatalf("CancelTask from pending: %v", err)
		}
		assertConflict(t, ts.CancelTask(ctx, task.ID), "CancelTask on cancelled")
		assertConflict(t, ts.FailTask(ctx, task.ID, "too late"), "FailTask on cancelled")
	})

	t.Run("fail rejected from terminal in_progress flow", func(t *testing.T) {
		task := createPending(t)
		claimed, err := ts.ClaimTask(ctx, task.ID, CoordinationTaskRoleResponder, "agent-1", "Agent One")
		if err != nil {
			t.Fatalf("ClaimTask: %v", err)
		}
		if claimed.Status != CoordinationTaskStatusAssigned {
			t.Fatalf("claimed status = %q, want assigned", claimed.Status)
		}
		if err := ts.MarkInProgress(ctx, task.ID); err != nil {
			t.Fatalf("MarkInProgress: %v", err)
		}
		if err := ts.CompleteTask(ctx, task.ID, map[string]any{"summary": "done"}); err != nil {
			t.Fatalf("CompleteTask from in_progress: %v", err)
		}
		assertConflict(t, ts.FailTask(ctx, task.ID, "after complete"), "FailTask on complete")
		assertConflict(t, ts.MarkInProgress(ctx, task.ID), "MarkInProgress on complete")
	})
}
