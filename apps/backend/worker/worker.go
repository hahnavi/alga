package worker

import (
	"context"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"

	"alga/logger"
	"alga/rabbitmq"
	"alga/trace"
)

func runTickerLoop(ctx context.Context, interval time.Duration, component string, tick func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Info("Worker started", "component", component)
	for {
		select {
		case <-ctx.Done():
			logger.Info("Worker stopped", "component", component)
			return
		case <-ticker.C:
			tick(ctx)
		}
	}
}

type Worker interface {
	Queue() string
	PrefetchCount() int
	Handle(ctx context.Context, d amqp.Delivery)
}

// WorkerSet manages all background consumers.
type WorkerSet struct {
	client             *rabbitmq.Client
	investigate        *InvestigateWorker
	escSweep           *EscalationSweepWorker
	actionItemSweep    *ActionItemSweepWorker
	heartbeatSweep     *HeartbeatSweepWorker
	stuckInvEscalation *StuckInvestigationEscalationWorker
	outbox             *OutboxWorker
	workers            []Worker
	wg                 sync.WaitGroup
	ctx                context.Context
	cancel             context.CancelFunc
}

// Config holds the dependencies needed by workers.
type Config struct {
	RabbitMQClient *rabbitmq.Client
	// Injected via setters
}

// NewWorkerSet creates all workers and declares topology.
func NewWorkerSet(client *rabbitmq.Client) (*WorkerSet, error) {
	if err := rabbitmq.DeclareTopology(client); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerSet{
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (ws *WorkerSet) RegisterWorker(w Worker) {
	ws.workers = append(ws.workers, w)
}

func (ws *WorkerSet) SetAlertWorker(w *AlertWorker) { ws.RegisterWorker(w) }
func (ws *WorkerSet) SetInvestigateWorker(w *InvestigateWorker) {
	ws.investigate = w
	ws.RegisterWorker(w)
}
func (ws *WorkerSet) SetEmailWorker(w *EmailWorker)           { ws.RegisterWorker(w) }
func (ws *WorkerSet) SetTriageWorker(w Worker)                { ws.RegisterWorker(w) }
func (ws *WorkerSet) SetIncidentWorker(w *IncidentWorker)     { ws.RegisterWorker(w) }
func (ws *WorkerSet) SetEscalationWorker(w *EscalationWorker) { ws.RegisterWorker(w) }
func (ws *WorkerSet) SetSLAWorker(w *SLAWorker)               { ws.RegisterWorker(w) }
func (ws *WorkerSet) SetNotificationDispatchWorker(w *NotificationDispatchWorker) {
	ws.RegisterWorker(w)
}
func (ws *WorkerSet) SetEscalationSweepWorker(w *EscalationSweepWorker) {
	ws.escSweep = w
}
func (ws *WorkerSet) SetActionItemSweepWorker(w *ActionItemSweepWorker) {
	ws.actionItemSweep = w
}
func (ws *WorkerSet) SetHeartbeatSweepWorker(w *HeartbeatSweepWorker) {
	ws.heartbeatSweep = w
}
func (ws *WorkerSet) SetStuckInvestigationEscalationWorker(w *StuckInvestigationEscalationWorker) {
	ws.stuckInvEscalation = w
}
func (ws *WorkerSet) SetICSWorker(w *ICSWorker)       { ws.RegisterWorker(w) }
func (ws *WorkerSet) SetOutboxWorker(w *OutboxWorker) { ws.outbox = w }

func (ws *WorkerSet) Start() {
	for _, w := range ws.workers {
		ws.wg.Add(1)
		w := w
		go func() {
			defer ws.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic in worker goroutine", "queue", w.Queue(), "error", r)
				}
			}()
			ws.consumeLoop(w.Queue(), w.PrefetchCount(), w.Handle)
		}()
	}
	if ws.escSweep != nil {
		ws.wg.Add(1)
		go func() {
			defer ws.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic in escalation sweep goroutine", "error", r)
				}
			}()
			ws.escSweep.Run(ws.ctx)
		}()
	}
	if ws.actionItemSweep != nil {
		ws.wg.Add(1)
		go func() {
			defer ws.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic in action item sweep goroutine", "error", r)
				}
			}()
			ws.actionItemSweep.Run(ws.ctx)
		}()
	}
	if ws.heartbeatSweep != nil {
		ws.wg.Add(1)
		go func() {
			defer ws.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic in heartbeat sweep goroutine", "error", r)
				}
			}()
			ws.heartbeatSweep.Run(ws.ctx)
		}()
	}
	if ws.stuckInvEscalation != nil {
		ws.wg.Add(1)
		go func() {
			defer ws.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic in stuck investigation escalation goroutine", "error", r)
				}
			}()
			ws.stuckInvEscalation.Run(ws.ctx)
		}()
	}
	if ws.outbox != nil {
		ws.wg.Add(1)
		go func() {
			defer ws.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic in outbox publisher goroutine", "error", r)
				}
			}()
			ws.outbox.Run(ws.ctx)
		}()
	}
	logger.Info("All workers started", "component", "worker-set")
}

// Stop signals workers to stop and waits for them to finish, including any
// in-flight handlers that have spawned goroutines (currently just the
// investigate worker, which tracks active processInvestigation calls in
// its own WaitGroup).
func (ws *WorkerSet) Stop() {
	logger.Info("Stopping workers", "component", "worker-set")
	ws.cancel()
	ws.wg.Wait()
	if ws.investigate != nil {
		ws.investigate.Stop()
	}
	logger.Info("All workers stopped", "component", "worker-set")
}

// consumeLoop reconnects and re-consumes on connection failure.
// prefetch sets the RabbitMQ QoS prefetch count (0 = unlimited). When set to
// match the worker's concurrency limit, RabbitMQ will only deliver that many
// unacked messages, acting as the flow-control gate (GitLab Runner model).
func (ws *WorkerSet) consumeLoop(queue string, prefetch int, handler func(context.Context, amqp.Delivery)) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		ch, err := ws.client.Channel()
		if err != nil {
			logger.Error("Worker failed to open channel", "component", "worker-set", "error", err)
			timer := time.NewTimer(backoff)
			select {
			case <-ws.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			backoff *= 2
			backoff = min(backoff, maxBackoff)
			continue
		}
		backoff = time.Second

		if prefetch > 0 {
			if qErr := ch.Qos(prefetch, 0, false); qErr != nil {
				logger.Warn("Failed to set QoS prefetch", "component", "worker-set", "prefetch", prefetch, "queue", queue, "error", qErr)
			}
		}

		deliveries, err := ch.Consume(queue, "", false, false, false, false, nil)
		if err != nil {
			logger.Error("Worker failed to consume queue", "component", "worker-set", "queue", queue, "error", err)
			_ = ch.Close()
			continue
		}

		logger.Info("Worker consuming from queue", "component", "worker-set", "queue", queue)

		done := false
		for !done {
			select {
			case <-ws.ctx.Done():
				_ = ch.Close()
				return
			case d, ok := <-deliveries:
				if !ok {
					done = true
					break
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("panic in handler; nacking for requeue", "queue", queue, "error", r)
							_ = d.Nack(false, true)
						}
					}()
					consumeCtx, span := startConsumeSpan(ws.ctx, d)
					defer span.End()
					handler(consumeCtx, d)
				}()
			}
		}
		_ = ch.Close()
	}
}

// startConsumeSpan extracts the W3C trace context from the delivery headers (when
// tracing is enabled) and starts a rabbitmq.consume span parented to it,
// surrounding the message Handle so worker processing appears as a child of the
// producing request's trace. When tracing is disabled it returns the parent ctx
// unchanged and the no-op span from context, so the span.End() call is harmless
// and the hot consume path pays nothing.
func startConsumeSpan(ctx context.Context, d amqp.Delivery) (context.Context, oteltrace.Span) {
	if !trace.Enabled() {
		return ctx, oteltrace.SpanFromContext(ctx)
	}
	parent := extractTraceContext(ctx, trace.Propagator(), d.Headers)
	ctx, span := trace.Tracer().Start(parent, "rabbitmq.consume",
		oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
		oteltrace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", d.Exchange),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.rabbitmq.routing_key", d.RoutingKey),
			attribute.String("messaging.message_id", d.MessageId),
		),
	)
	return ctx, span
}

// extractTraceContext reads the W3C trace context from AMQP delivery headers
// using the provided propagator. It is pure so the extraction logic is unit
// testable independent of whether tracing is enabled.
func extractTraceContext(ctx context.Context, propagator propagation.TextMapPropagator, headers amqp.Table) context.Context {
	carrier := rabbitmq.NewAMQPCarrier(headers)
	return propagator.Extract(ctx, carrier)
}
