package worker

import (
	"context"
	"time"

	"alga/logger"
	"alga/rabbitmq"
	"alga/store"
)

// outboxTick is how often the outbox publisher scans for unpublished rows.
const outboxTick = 5 * time.Second

// OutboxPublisher abstracts the publish step so the worker can run without a
// live RabbitMQ connection in tests (and so it stays decoupled from the
// concrete *rabbitmq.Publisher).
type OutboxPublisher interface {
	PublishRaw(ctx context.Context, exchange, routingKey string, body []byte) error
}

// OutboxWorker drains the outbox table: it fetches unpublished rows, republishes
// their stored payload to the exchange/routing key recorded alongside them, and
// marks each row published on success or schedules a retry (W4 RetrySchedule)
// on failure. Rows that exhaust MaxOutboxRetries become terminal-failed (the
// outbox's DLQ-equivalent). It is idempotent: the row is published at most once
// because MarkPublished is committed before the next scan, and the payload
// carries the original W5 EventID consumers dedupe on.
type OutboxWorker struct {
	store     store.OutboxStore
	publisher OutboxPublisher
	interval  time.Duration
	retention time.Duration
	// pruneEvery bounds how often the worker prunes published rows. Pruning is
	// cheap but needless every tick, so we do it once per pruneEvery scans.
	pruneEvery int
	ticks      int
}

// NewOutboxWorker builds the outbox drainer. retention is how long a published
// row is kept before pruning (0 disables pruning); interval is the scan period.
func NewOutboxWorker(s store.OutboxStore, p OutboxPublisher, interval, retention time.Duration) *OutboxWorker {
	if interval <= 0 {
		interval = outboxTick
	}
	if retention < 0 {
		retention = 0
	}
	return &OutboxWorker{store: s, publisher: p, interval: interval, retention: retention, pruneEvery: 12}
}

// Run loops on the configured interval until ctx is cancelled.
func (w *OutboxWorker) Run(ctx context.Context) {
	runTickerLoop(ctx, w.interval, "outbox-publisher", w.tick)
}

func (w *OutboxWorker) tick(ctx context.Context) {
	if w.store == nil || w.publisher == nil {
		return
	}

	w.ticks++
	if w.retention > 0 && w.pruneEvery > 0 && w.ticks%w.pruneEvery == 0 {
		cutoff := time.Now().UTC().Add(-w.retention)
		if n, err := w.store.PrunePublished(ctx, cutoff); err != nil {
			logger.Error("outbox: retention sweep failed", "component", "outbox-publisher", "error", err)
		} else if n > 0 {
			logger.Info("outbox: pruned published rows", "component", "outbox-publisher", "count", n)
		}
	}

	rows, err := w.store.FetchUnpublished(ctx, 100)
	if err != nil {
		logger.Error("outbox: failed to fetch unpublished", "component", "outbox-publisher", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	for _, row := range rows {
		w.publishOne(ctx, row)
	}
}

func (w *OutboxWorker) publishOne(ctx context.Context, row store.OutboxRecord) {
	pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := w.publisher.PublishRaw(pubCtx, row.Exchange, row.RoutingKey, row.Payload); err != nil {
		logger.Warn("outbox: publish failed; scheduling retry",
			"component", "outbox-publisher", "outbox_id", row.ID, "event_id", row.EventID,
			"retry_count", row.RetryCount, "error", err)

		if row.RetryCount+1 >= store.MaxOutboxRetries {
			if mErr := w.store.MarkFailed(pubCtx, row.ID); mErr != nil {
				logger.Error("outbox: failed to mark terminal", "component", "outbox-publisher", "outbox_id", row.ID, "error", mErr)
			}
			return
		}
		if mErr := w.store.IncrementRetry(pubCtx, row.ID, w.nextAttempt(row.RetryCount+1)); mErr != nil {
			logger.Error("outbox: failed to record retry", "component", "outbox-publisher", "outbox_id", row.ID, "error", mErr)
		}
		return
	}

	now := time.Now().UTC()
	if err := w.store.MarkPublished(pubCtx, row.ID, now); err != nil {
		logger.Error("outbox: failed to mark published", "component", "outbox-publisher", "outbox_id", row.ID, "event_id", row.EventID, "error", err)
		return
	}
	logger.Info("outbox: published event", "component", "outbox-publisher", "outbox_id", row.ID, "event_id", row.EventID, "event_type", row.EventType)
}

// nextAttempt returns the backoff deadline for the given 1-based retry attempt,
// reusing the authoritative W4 RetrySchedule. It clamps to the last stage so a
// malformed attempt count never panics or schedules infinitely far out.
func (w *OutboxWorker) nextAttempt(retryCount int) time.Time {
	stage := retryCount - 1
	if stage < 0 {
		stage = 0
	}
	if stage >= len(rabbitmq.RetrySchedule) {
		stage = len(rabbitmq.RetrySchedule) - 1
	}
	return time.Now().UTC().Add(rabbitmq.RetrySchedule[stage])
}
