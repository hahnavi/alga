package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/store"
)

// fakeOutboxPublisher records publish attempts and can be made to fail the
// first N attempts, letting the test exercise the retry/terminal path without
// a live RabbitMQ.
type fakeOutboxPublisher struct {
	mu     sync.Mutex
	pub    [][]byte
	calls  int
	failN  int
	lastEx string
	lastRK string
}

func (f *fakeOutboxPublisher) PublishRaw(ctx context.Context, exchange, routingKey string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastEx = exchange
	f.lastRK = routingKey
	if f.calls <= f.failN {
		return errors.New("simulated publish failure")
	}
	b := make([]byte, len(body))
	copy(b, body)
	f.pub = append(f.pub, b)
	return nil
}

func (f *fakeOutboxPublisher) publishedCopy() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.pub))
	copy(out, f.pub)
	return out
}

// fakeOutboxStore is an in-memory OutboxStore used to unit-test the worker's
// orchestration (fetch -> publish -> mark) without Postgres. The real store's
// SQL behavior is covered by store/outbox_test.go.
type fakeOutboxStore struct {
	mu    sync.Mutex
	rows  map[uuid.UUID]store.OutboxRecord
	order []uuid.UUID
}

func newFakeOutboxStore() *fakeOutboxStore {
	return &fakeOutboxStore{rows: make(map[uuid.UUID]store.OutboxRecord)}
}

// rowsSet inserts (or replaces) a row directly, bypassing the enqueue path so
// tests can seed already-published or terminal rows.
func (s *fakeOutboxStore) rowsSet(r store.OutboxRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[r.ID] = r
	return nil
}

func (s *fakeOutboxStore) InsertOutbox(ctx context.Context, tx *ent.Tx, eventType, aggregateID, exchange, routingKey string, payload []byte, eventID string) error {
	return s.enqueue(eventType, aggregateID, exchange, routingKey, payload, eventID)
}

func (s *fakeOutboxStore) EnqueueOutbox(ctx context.Context, eventType, aggregateID, exchange, routingKey string, payload []byte, eventID string) error {
	return s.enqueue(eventType, aggregateID, exchange, routingKey, payload, eventID)
}

func (s *fakeOutboxStore) enqueue(eventType, aggregateID, exchange, routingKey string, payload []byte, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := uuid.New()
	rec := store.OutboxRecord{
		ID:          id,
		EventType:   eventType,
		AggregateID: aggregateID,
		Exchange:    exchange,
		RoutingKey:  routingKey,
		Payload:     payload,
		Status:      store.OutboxStatusPending,
		EventID:     eventID,
		RetryCount:  0,
		CreatedAt:   time.Now().UTC(),
	}
	s.rows[id] = rec
	s.order = append(s.order, id)
	return nil
}

func (s *fakeOutboxStore) FetchUnpublished(ctx context.Context, limit int) ([]store.OutboxRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	var out []store.OutboxRecord
	for _, id := range s.order {
		r := s.rows[id]
		due := false
		switch r.Status {
		case store.OutboxStatusPending:
			due = r.NextAttemptAt == nil || !r.NextAttemptAt.After(now)
		case store.OutboxStatusFailed:
			due = r.RetryCount < store.MaxOutboxRetries && (r.NextAttemptAt == nil || !r.NextAttemptAt.After(now))
		}
		if due {
			out = append(out, r)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *fakeOutboxStore) MarkPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rows[id]
	r.Status = store.OutboxStatusPublished
	pa := publishedAt
	r.PublishedAt = &pa
	s.rows[id] = r
	return nil
}

func (s *fakeOutboxStore) IncrementRetry(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rows[id]
	r.RetryCount++
	r.Status = store.OutboxStatusFailed
	if nextAttemptAt.IsZero() {
		r.NextAttemptAt = nil
	} else {
		na := nextAttemptAt
		r.NextAttemptAt = &na
	}
	s.rows[id] = r
	return nil
}

func (s *fakeOutboxStore) MarkFailed(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rows[id]
	// Terminal: set retry_count to the max so FetchUnpublished excludes it.
	r.RetryCount = store.MaxOutboxRetries
	r.Status = store.OutboxStatusFailed
	r.NextAttemptAt = nil
	s.rows[id] = r
	return nil
}

func (s *fakeOutboxStore) PrunePublished(ctx context.Context, olderThan time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	for id, r := range s.rows {
		if r.Status == store.OutboxStatusPublished && r.PublishedAt != nil && r.PublishedAt.Before(olderThan) {
			delete(s.rows, id)
			n++
		}
	}
	return n, nil
}

func (s *fakeOutboxStore) GetOutbox(ctx context.Context, id uuid.UUID) (*store.OutboxRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[id]
	if !ok {
		return nil, nil
	}
	cp := r
	return &cp, nil
}

func TestOutboxWorkerPublishAndMarkPublished(t *testing.T) {
	s := newFakeOutboxStore()
	fp := &fakeOutboxPublisher{}
	w := NewOutboxWorker(s, fp, time.Second, 7*24*time.Hour)

	if err := s.EnqueueOutbox(context.Background(), "alert.received", "agg-1", "ex", "rk", []byte(`{"a":1}`), "evt-1"); err != nil {
		t.Fatalf("EnqueueOutbox: %v", err)
	}

	w.tick(context.Background())

	pub := fp.publishedCopy()
	if len(pub) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(pub))
	}
	if string(pub[0]) != `{"a":1}` {
		t.Fatalf("published body = %q, want %q", pub[0], `{"a":1}`)
	}
	if fp.lastEx != "ex" || fp.lastRK != "rk" {
		t.Fatalf("published to ex=%q rk=%q, want ex=ex rk=rk", fp.lastEx, fp.lastRK)
	}

	due, err := s.FetchUnpublished(context.Background(), 10)
	if err != nil {
		t.Fatalf("FetchUnpublished: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("expected 0 due rows after publish, got %d", len(due))
	}
}

func TestOutboxWorkerRetriesThenTerminal(t *testing.T) {
	s := newFakeOutboxStore()
	fp := &fakeOutboxPublisher{failN: 1000} // always fail
	w := NewOutboxWorker(s, fp, time.Second, 7*24*time.Hour)

	if err := s.EnqueueOutbox(context.Background(), "alert.received", "agg", "ex", "rk", []byte(`{}`), "evt-r"); err != nil {
		t.Fatalf("EnqueueOutbox: %v", err)
	}
	due, _ := s.FetchUnpublished(context.Background(), 10)
	id := due[0].ID

	// Drive publishOne directly with increasing retry_count to exercise the
	// backoff/terminal state machine deterministically (bypassing the
	// next_attempt_at gate).
	for attempt := 0; attempt < store.MaxOutboxRetries; attempt++ {
		rec, err := s.GetOutbox(context.Background(), id)
		if err != nil {
			t.Fatalf("GetOutbox: %v", err)
		}
		w.publishOne(context.Background(), store.OutboxRecord{
			ID:         id,
			Exchange:   "ex",
			RoutingKey: "rk",
			Payload:    []byte(`{}`),
			EventID:    "evt-r",
			RetryCount: rec.RetryCount,
		})
	}

	rec, err := s.GetOutbox(context.Background(), id)
	if err != nil {
		t.Fatalf("GetOutbox: %v", err)
	}
	if rec.RetryCount != store.MaxOutboxRetries {
		t.Fatalf("retry_count = %d, want %d", rec.RetryCount, store.MaxOutboxRetries)
	}
	if rec.Status != store.OutboxStatusFailed {
		t.Fatalf("status = %q, want failed (terminal)", rec.Status)
	}
	if rec.NextAttemptAt != nil {
		t.Fatal("terminal row must have nil next_attempt_at")
	}
	if len(fp.publishedCopy()) != 0 {
		t.Fatal("expected zero successful publishes")
	}
}

func TestOutboxWorkerIgnoresTerminalRows(t *testing.T) {
	s := newFakeOutboxStore()
	fp := &fakeOutboxPublisher{}
	w := NewOutboxWorker(s, fp, time.Second, 7*24*time.Hour)

	if err := s.EnqueueOutbox(context.Background(), "alert.received", "agg", "ex", "rk", []byte(`{}`), "evt-t"); err != nil {
		t.Fatalf("EnqueueOutbox: %v", err)
	}
	due, _ := s.FetchUnpublished(context.Background(), 10)
	if err := s.MarkFailed(context.Background(), due[0].ID); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	// A terminal-failed row must not be (re)published.
	w.tick(context.Background())
	if len(fp.publishedCopy()) != 0 {
		t.Fatal("terminal row must not be published")
	}
}

func TestOutboxWorkerRespectsBackoff(t *testing.T) {
	s := newFakeOutboxStore()
	fp := &fakeOutboxPublisher{failN: 1000}
	w := NewOutboxWorker(s, fp, time.Second, 7*24*time.Hour)

	if err := s.EnqueueOutbox(context.Background(), "alert.received", "agg", "ex", "rk", []byte(`{}`), "evt-b"); err != nil {
		t.Fatalf("EnqueueOutbox: %v", err)
	}
	due, _ := s.FetchUnpublished(context.Background(), 10)
	// One failed attempt schedules the next for the future.
	w.publishOne(context.Background(), store.OutboxRecord{
		ID:         due[0].ID,
		Exchange:   "ex",
		RoutingKey: "rk",
		Payload:    []byte(`{}`),
		EventID:    "evt-b",
		RetryCount: 0,
	})

	// Immediately after, the row is backed off and must not be due again.
	due2, err := s.FetchUnpublished(context.Background(), 10)
	if err != nil {
		t.Fatalf("FetchUnpublished: %v", err)
	}
	for _, r := range due2 {
		if r.ID == due[0].ID {
			t.Fatal("backed-off row must not be due until its next_attempt_at elapses")
		}
	}
}

func TestOutboxWorkerRetentionPrunesPublished(t *testing.T) {
	s := newFakeOutboxStore()
	fp := &fakeOutboxPublisher{}
	w := NewOutboxWorker(s, fp, time.Second, 7*24*time.Hour)
	w.pruneEvery = 1 // prune on every tick so the test is deterministic

	// Published row older than the retention window.
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	if err := s.rowsSet(store.OutboxRecord{
		ID:          mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		EventType:   "alert.received",
		Exchange:    "ex",
		RoutingKey:  "rk",
		Payload:     []byte(`{}`),
		EventID:     "evt-old",
		Status:      store.OutboxStatusPublished,
		PublishedAt: &old,
		CreatedAt:   old,
	}); err != nil {
		t.Fatalf("seed published row: %v", err)
	}

	// Published row inside the window must survive.
	recent := time.Now().UTC().Add(-1 * time.Hour)
	if err := s.rowsSet(store.OutboxRecord{
		ID:          mustUUID(t, "00000000-0000-0000-0000-000000000002"),
		EventType:   "alert.received",
		Exchange:    "ex",
		RoutingKey:  "rk",
		Payload:     []byte(`{}`),
		EventID:     "evt-recent",
		Status:      store.OutboxStatusPublished,
		PublishedAt: &recent,
		CreatedAt:   recent,
	}); err != nil {
		t.Fatalf("seed recent row: %v", err)
	}

	// Terminal-failed rows must never be pruned even if old.
	failedOld := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if err := s.rowsSet(store.OutboxRecord{
		ID:         mustUUID(t, "00000000-0000-0000-0000-000000000003"),
		EventType:  "alert.received",
		Exchange:   "ex",
		RoutingKey: "rk",
		Payload:    []byte(`{}`),
		EventID:    "evt-failed",
		Status:     store.OutboxStatusFailed,
		CreatedAt:  failedOld,
	}); err != nil {
		t.Fatalf("seed failed row: %v", err)
	}

	w.tick(context.Background())

	if _, ok := s.rows[mustUUID(t, "00000000-0000-0000-0000-000000000001")]; ok {
		t.Fatal("old published row must be pruned")
	}
	if _, ok := s.rows[mustUUID(t, "00000000-0000-0000-0000-000000000002")]; !ok {
		t.Fatal("recent published row must be retained")
	}
	if _, ok := s.rows[mustUUID(t, "00000000-0000-0000-0000-000000000003")]; !ok {
		t.Fatal("terminal-failed row must never be pruned")
	}
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return id
}
