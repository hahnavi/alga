package store

import (
	"context"
	"testing"
	"time"
)

func TestOutboxEnqueueFetchAndPublish(t *testing.T) {
	client := newTestEntClient(t)
	s := newPGOutboxStore(client)

	if err := s.EnqueueOutbox(context.Background(), "alert.received", "agg-1", "ex", "rk", []byte(`{"a":1}`), "evt-1"); err != nil {
		t.Fatalf("EnqueueOutbox: %v", err)
	}

	due, err := s.FetchUnpublished(context.Background(), 10)
	if err != nil {
		t.Fatalf("FetchUnpublished: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected 1 due row, got %d", len(due))
	}
	if due[0].EventType != "alert.received" || due[0].Exchange != "ex" || string(due[0].Payload) != `{"a":1}` {
		t.Fatalf("unexpected row: %+v", due[0])
	}
	if due[0].Status != OutboxStatusPending {
		t.Fatalf("status = %q, want pending", due[0].Status)
	}

	now := time.Now().UTC()
	if err := s.MarkPublished(context.Background(), due[0].ID, now); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	due2, err := s.FetchUnpublished(context.Background(), 10)
	if err != nil {
		t.Fatalf("FetchUnpublished after publish: %v", err)
	}
	if len(due2) != 0 {
		t.Fatalf("expected 0 due rows after publish, got %d", len(due2))
	}

	rec, err := s.GetOutbox(context.Background(), due[0].ID)
	if err != nil {
		t.Fatalf("GetOutbox: %v", err)
	}
	if rec == nil {
		t.Fatal("expected published row to be fetchable by id")
	}
	if rec.Status != OutboxStatusPublished {
		t.Fatalf("status = %q, want published", rec.Status)
	}
	if rec.PublishedAt == nil {
		t.Fatal("expected published_at to be set")
	}
	if diff := rec.PublishedAt.Sub(now); diff < -2*time.Second || diff > 2*time.Second {
		t.Fatalf("published_at = %v, want ~%v", rec.PublishedAt, now)
	}
}

func TestOutboxInsertInTxRollbackKeepsNothing(t *testing.T) {
	client := newTestEntClient(t)
	s := newPGOutboxStore(client)

	ctx := context.Background()
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	// Intentionally roll back (do not commit).
	defer rollbackTx(tx)

	if err := s.InsertOutbox(ctx, tx, "alert.received", "agg", "ex", "rk", []byte(`{}`), "evt-x"); err != nil {
		t.Fatalf("InsertOutbox: %v", err)
	}
	// After rollback the row must not be visible.
	tx.Rollback() //nolint:errcheck

	due, err := s.FetchUnpublished(ctx, 10)
	if err != nil {
		t.Fatalf("FetchUnpublished: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("expected rolled-back row to be absent, got %d", len(due))
	}
}

func TestOutboxIncrementRetryAndTerminal(t *testing.T) {
	client := newTestEntClient(t)
	s := newPGOutboxStore(client)

	if err := s.EnqueueOutbox(context.Background(), "alert.received", "agg", "ex", "rk", []byte(`{}`), "evt-r"); err != nil {
		t.Fatalf("EnqueueOutbox: %v", err)
	}
	due, _ := s.FetchUnpublished(context.Background(), 10)
	id := due[0].ID

	backoff := time.Now().UTC().Add(time.Minute)
	if err := s.IncrementRetry(context.Background(), id, backoff); err != nil {
		t.Fatalf("IncrementRetry: %v", err)
	}
	rec, _ := s.GetOutbox(context.Background(), id)
	if rec == nil || rec.RetryCount != 1 {
		t.Fatalf("retry_count = %v, want 1", recRetry(rec))
	}
	if rec.Status != OutboxStatusFailed {
		t.Fatalf("status = %q, want failed", rec.Status)
	}
	if rec.NextAttemptAt == nil {
		t.Fatal("expected next_attempt_at to be set on a retryable failure")
	}

	// Exhaust the retry budget the same way the worker does: retryable
	// failures via IncrementRetry until one short of max, then a terminal
	// MarkFailed (which clears next_attempt_at).
	for rec != nil && rec.RetryCount < MaxOutboxRetries-1 {
		if err := s.IncrementRetry(context.Background(), id, time.Now().UTC().Add(time.Minute)); err != nil {
			t.Fatalf("IncrementRetry: %v", err)
		}
		rec, _ = s.GetOutbox(context.Background(), id)
	}
	if err := s.MarkFailed(context.Background(), id); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	rec, _ = s.GetOutbox(context.Background(), id)
	if rec.RetryCount != MaxOutboxRetries {
		t.Fatalf("retry_count = %d, want %d", rec.RetryCount, MaxOutboxRetries)
	}
	if rec.NextAttemptAt != nil {
		t.Fatal("terminal row must have nil next_attempt_at")
	}
	// Terminal-failed rows are never returned as due.
	due2, _ := s.FetchUnpublished(context.Background(), 10)
	for _, r := range due2 {
		if r.ID == id {
			t.Fatal("terminal-failed row must not be returned by FetchUnpublished")
		}
	}
}

func TestOutboxMarkFailedTerminal(t *testing.T) {
	client := newTestEntClient(t)
	s := newPGOutboxStore(client)

	if err := s.EnqueueOutbox(context.Background(), "alert.received", "agg", "ex", "rk", []byte(`{}`), "evt-f"); err != nil {
		t.Fatalf("EnqueueOutbox: %v", err)
	}
	due, _ := s.FetchUnpublished(context.Background(), 10)
	id := due[0].ID

	if err := s.MarkFailed(context.Background(), id); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	rec, _ := s.GetOutbox(context.Background(), id)
	if rec == nil || rec.Status != OutboxStatusFailed {
		t.Fatalf("status = %v, want failed", rec)
	}
	if rec.RetryCount != MaxOutboxRetries {
		t.Fatalf("retry_count = %d, want %d (terminal)", rec.RetryCount, MaxOutboxRetries)
	}
}

func recRetry(r *OutboxRecord) int {
	if r == nil {
		return -1
	}
	return r.RetryCount
}
