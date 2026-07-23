package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/outbox"
)

// Outbox status values for the outbox state machine.
const (
	OutboxStatusPending   = "pending"
	OutboxStatusPublished = "published"
	OutboxStatusFailed    = "failed"

	// MaxOutboxRetries bounds how many times the outbox publisher worker
	// attempts to publish a row before marking it terminal-failed
	// (DLQ-equivalent). It aligns with the W4 RetrySchedule length so every
	// backoff stage maps to exactly one retry attempt.
	MaxOutboxRetries = 4
)

// OutboxRecord is the application-level view of an outbox row.
type OutboxRecord struct {
	ID            uuid.UUID  `json:"id"`
	EventType     string     `json:"event_type"`
	AggregateID   string     `json:"aggregate_id"`
	Exchange      string     `json:"exchange"`
	RoutingKey    string     `json:"routing_key"`
	Payload       []byte     `json:"-"`
	Status        string     `json:"status"`
	EventID       string     `json:"event_id"`
	RetryCount    int        `json:"retry_count"`
	CreatedAt     time.Time  `json:"created_at"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
}

// OutboxStore is the durable event log backing the transactional outbox.
type OutboxStore interface {
	// InsertOutbox writes an outbox row inside the caller's business
	// transaction. Use it when an outbox event must commit atomically with
	// other domain writes (the canonical outbox pattern).
	InsertOutbox(ctx context.Context, tx *ent.Tx, eventType, aggregateID, exchange, routingKey string, payload []byte, eventID string) error
	// EnqueueOutbox opens its own transaction and inserts the outbox row. Use
	// it when there is no surrounding business transaction (e.g. the alert
	// webhook hot path, which commits the outbox row as its durable write).
	EnqueueOutbox(ctx context.Context, eventType, aggregateID, exchange, routingKey string, payload []byte, eventID string) error
	// FetchUnpublished returns up to limit rows, oldest-first, that are due to
	// be published: status=pending (due now), or status=failed with
	// retry_count < MaxOutboxRetries and a next_attempt_at that has elapsed (or
	// is unset). Terminal-failed rows are never returned.
	FetchUnpublished(ctx context.Context, limit int) ([]OutboxRecord, error)
	// MarkPublished records a successful publish.
	MarkPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error
	// IncrementRetry records a failed publish attempt. It bumps retry_count and
	// sets status=failed. When the new retry_count reaches MaxOutboxRetries the
	// row becomes terminal (next_attempt_at cleared); otherwise it is scheduled
	// for another attempt at nextAttemptAt.
	IncrementRetry(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time) error
	// MarkFailed terminal-fails a row regardless of retry_count (e.g. a poison
	// payload that can never be published).
	MarkFailed(ctx context.Context, id uuid.UUID) error
	// GetOutbox returns a single outbox row by id, or (nil, nil) if absent.
	GetOutbox(ctx context.Context, id uuid.UUID) (*OutboxRecord, error)
	// PrunePublished deletes published outbox rows older than olderThan and
	// returns the number removed. Published rows are only needed until the
	// outbox worker confirms delivery, after which they are dead weight;
	// pruning keeps the table bounded. Terminal-failed rows are intentionally
	// retained for operator inspection (they signal a poison payload that can
	// never be published).
	PrunePublished(ctx context.Context, olderThan time.Time) (int, error)
}

type pgOutboxStore struct {
	pgStoreBase
}

func newPGOutboxStore(client *ent.Client) OutboxStore {
	return &pgOutboxStore{pgStoreBase{client: client}}
}

func (s *pgOutboxStore) InsertOutbox(ctx context.Context, tx *ent.Tx, eventType, aggregateID, exchange, routingKey string, payload []byte, eventID string) error {
	if tx == nil {
		return errors.New("nil transaction for outbox insert")
	}
	if _, err := tx.Outbox.Create().
		SetEventType(eventType).
		SetAggregateID(aggregateID).
		SetExchange(exchange).
		SetRoutingKey(routingKey).
		SetPayload(payload).
		SetEventID(eventID).
		SetStatus(OutboxStatusPending).
		Save(ctx); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

func (s *pgOutboxStore) EnqueueOutbox(ctx context.Context, eventType, aggregateID, exchange, routingKey string, payload []byte, eventID string) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin outbox tx: %w", err)
	}
	defer rollbackTx(tx)
	if err := s.InsertOutbox(ctx, tx, eventType, aggregateID, exchange, routingKey, payload, eventID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit outbox tx: %w", err)
	}
	return nil
}

func (s *pgOutboxStore) FetchUnpublished(ctx context.Context, limit int) ([]OutboxRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	limit = min(limit, 500)
	now := time.Now().UTC()

	rows, err := s.client.Outbox.Query().
		Where(
			outbox.Or(
				outbox.And(
					outbox.StatusEQ(outbox.Status(OutboxStatusPending)),
					outbox.Or(outbox.NextAttemptAtIsNil(), outbox.NextAttemptAtLTE(now)),
				),
				outbox.And(
					outbox.StatusEQ(outbox.Status(OutboxStatusFailed)),
					outbox.RetryCountLT(MaxOutboxRetries),
					outbox.Or(outbox.NextAttemptAtIsNil(), outbox.NextAttemptAtLTE(now)),
				),
			),
		).
		Order(ent.Asc(outbox.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch unpublished outbox: %w", err)
	}

	out := make([]OutboxRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, toOutboxRecord(r))
	}
	return out, nil
}

func (s *pgOutboxStore) MarkPublished(ctx context.Context, id uuid.UUID, publishedAt time.Time) error {
	if _, err := s.client.Outbox.UpdateOneID(id).
		SetStatus(OutboxStatusPublished).
		SetPublishedAt(publishedAt).
		Save(ctx); err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

func (s *pgOutboxStore) IncrementRetry(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time) error {
	upd := s.client.Outbox.UpdateOneID(id).
		AddRetryCount(1).
		SetStatus(OutboxStatusFailed)
	if nextAttemptAt.IsZero() {
		upd = upd.ClearNextAttemptAt()
	} else {
		upd = upd.SetNextAttemptAt(nextAttemptAt)
	}
	if _, err := upd.Save(ctx); err != nil {
		return fmt.Errorf("increment outbox retry: %w", err)
	}
	return nil
}

func (s *pgOutboxStore) MarkFailed(ctx context.Context, id uuid.UUID) error {
	// Set retry_count to the maximum so the row is terminal: FetchUnpublished
	// only returns failed rows with retry_count < Max, so a terminal row is
	// never re-fetched (DLQ-equivalent).
	if _, err := s.client.Outbox.UpdateOneID(id).
		SetRetryCount(MaxOutboxRetries).
		SetStatus(OutboxStatusFailed).
		ClearNextAttemptAt().
		Save(ctx); err != nil {
		return fmt.Errorf("mark outbox failed: %w", err)
	}
	return nil
}

func (s *pgOutboxStore) GetOutbox(ctx context.Context, id uuid.UUID) (*OutboxRecord, error) {
	r, err := s.client.Outbox.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get outbox: %w", err)
	}
	rec := toOutboxRecord(r)
	return &rec, nil
}

// PrunePublished deletes published outbox rows older than olderThan and returns
// the number removed. Published rows are only needed until the outbox worker
// confirms delivery, after which they are dead weight; pruning keeps the table
// bounded. Terminal-failed rows are intentionally retained for operator
// inspection (they signal a poison payload that can never be published).
func (s *pgOutboxStore) PrunePublished(ctx context.Context, olderThan time.Time) (int, error) {
	n, err := s.client.Outbox.Delete().
		Where(outbox.StatusEQ(outbox.Status(OutboxStatusPublished)), outbox.CreatedAtLT(olderThan)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("prune published outbox: %w", err)
	}
	return n, nil
}

func toOutboxRecord(r *ent.Outbox) OutboxRecord {
	rec := OutboxRecord{
		ID:          r.ID,
		EventType:   r.EventType,
		AggregateID: r.AggregateID,
		Exchange:    r.Exchange,
		RoutingKey:  r.RoutingKey,
		Payload:     r.Payload,
		Status:      string(r.Status),
		EventID:     r.EventID,
		RetryCount:  r.RetryCount,
		CreatedAt:   r.CreatedAt,
	}
	if r.PublishedAt != nil {
		pa := *r.PublishedAt
		rec.PublishedAt = &pa
	}
	if r.NextAttemptAt != nil {
		na := *r.NextAttemptAt
		rec.NextAttemptAt = &na
	}
	return rec
}
