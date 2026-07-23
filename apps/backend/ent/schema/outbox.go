package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Outbox is the durable event log for the transactional outbox pattern (W6).
// A business transaction INSERTs a row here alongside its domain writes; the
// Outbox Publisher worker (worker/outbox.go) later publishes the stored payload
// to RabbitMQ and marks the row published. This guarantees no event is lost if
// the publisher or broker is unavailable between the DB commit and the publish.
//
// The payload is the exact, already-marshaled AMQP message body (including the
// W5 EventEnvelope with its EventID), so the worker republishes it verbatim and
// consumers keep their original idempotency key. `exchange`/`routing_key` record
// where the message must go, so the worker needs no per-event-type switch.
type Outbox struct {
	ent.Schema
}

func (Outbox) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).Unique(),
		// event_type is the W5 aggregate.action wire name (e.g. "alert.received").
		field.String("event_type").NotEmpty(),
		// aggregate_id is the domain aggregate the event is about (empty when
		// not yet known, e.g. at alert ingest before an alert_number exists).
		field.String("aggregate_id").Default(""),
		// exchange + routing_key are where the stored payload must be published.
		field.String("exchange").NotEmpty(),
		field.String("routing_key").Default(""),
		// payload is the raw, already-marshaled AMQP message body (JSON bytes
		// carrying the W5 envelope). Never contains secrets — it is the message
		// only.
		field.Bytes("payload"),
		// status: pending (not yet published) -> published (success, terminal),
		// or failed (gave up after max retries, terminal / DLQ-equivalent).
		field.Enum("status").Values("pending", "published", "failed").Default("pending"),
		// event_id is the denormalized W5 EventEnvelope.EventID, kept for
		// idempotency lookups and debugging. The canonical value lives inside
		// the payload envelope; this column is an indexable mirror.
		field.String("event_id").Default(""),
		// retry_count counts failed publish attempts. Terminal-failed rows have
		// retry_count >= MaxOutboxRetries.
		field.Int("retry_count").Default(0),
		field.Time("created_at").Default(timeNow),
		// published_at is set when the row is successfully published.
		field.Time("published_at").Optional().Nillable(),
		// next_attempt_at gates retry eligibility: a failed row is only fetched
		// again once this time has passed (W4 RetrySchedule backoff). Nil means
		// "due now" or "terminal" (terminal rows also have retry_count >= max).
		field.Time("next_attempt_at").Optional().Nillable(),
	}
}

func (Outbox) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "next_attempt_at"),
		index.Fields("aggregate_id"),
		index.Fields("event_id"),
	}
}
