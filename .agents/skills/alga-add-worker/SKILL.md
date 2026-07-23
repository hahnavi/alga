---
name: alga-add-worker
description: Use when adding or changing RabbitMQ background consumers, async jobs, retry queues, sweepers, or worker lifecycle wiring in Alga.
priority: P1
tags: [backend, go, rabbitmq, worker, async]
---

# Add a RabbitMQ Worker

Use this for background consumers, retry/dead-letter flows, async publishers, and worker lifecycle wiring. For non-RabbitMQ scheduled logic, still follow the idempotency, logging, and shutdown rules here.

Before editing, state the queue, retry/dead-letter policy, idempotency key, ack/nack behavior, lifecycle wiring, and tests you expect to add. For alert, investigation, incident, scheduler, RabbitMQ, or Valkey behavior, also use `alga-domain-invariants`.

## Check First

- Worker interface and registration: `apps/backend/worker/worker.go`.
- Retry helper: `apps/backend/worker/retry.go`.
- Existing workers near the domain: `apps/backend/worker/` and `apps/backend/triage/worker.go`.
- RabbitMQ topology: `apps/backend/rabbitmq/topology.go`.
- Publisher and messages: `apps/backend/rabbitmq/*.go`.
- App wiring: `apps/backend/app/wire.go`.

## Core Rules

- Implement `Queue()`, `PrefetchCount()`, and `Handle(ctx, delivery)` for RabbitMQ consumers.
- Register with `WorkerSet.RegisterWorker` through a `SetXxxWorker` method.
- Let `WorkerSet.Start()` and `WorkerSet.Stop()` own normal consumer lifecycle. Add per-worker goroutines only when the worker truly needs them, and stop them with context cancellation.
- Use structured logger calls with a stable `component` value.
- Ack only after durable success. Nack malformed messages without requeue unless the local pattern says otherwise.
- Use `RetryCount`, not `Retries`.
- Pick idempotency keys deliberately. Use `DedupeKey` only when the message schema defines one; otherwise use a stable domain identifier or explicitly document why idempotency is not needed.

## RabbitMQ Topology

- Add exchange, process queue, retry queues, routing keys, and max retry constants in `rabbitmq/topology.go`.
- Declare queues and bindings in `declareTopology`.
- Add DLX/retry queues when the worker retries.
- Keep queue names under the existing `alga.<domain>.<purpose>` convention.

## Message and Publisher

- Add message structs near related `rabbitmq/*messages.go` files.
- Include `RetryCount int \`json:"retry_count"\`` for retryable messages.
- Add `UnmarshalXxxMessage` and publisher methods near local patterns.
- Use URL-safe/string-safe IDs in messages; avoid embedding large payloads when IDs can be loaded by the worker.

## Worker Shape

```go
func (w *XxxWorker) Handle(ctx context.Context, d amqp.Delivery) {
    msg, err := rabbitmq.UnmarshalXxxMessage(d)
    if err != nil {
        logger.Error("failed to unmarshal xxx message", "component", "xxx-worker", "error", err)
        _ = d.Nack(false, false)
        return
    }

    if err := w.process(ctx, msg); err != nil {
        logger.Error("failed to process xxx message", "component", "xxx-worker", "error", err, "retry", msg.RetryCount)
        w.scheduleRetryOrDeadLetter(ctx, msg, d, "process", err)
        return
    }
    _ = d.Ack(false)
}
```

Adapt retry scheduling to the message schema and nearest worker. The shared `retryOrDeadLetter` helper is preferred when the domain has a retry publisher and idempotency key.

## Wiring Checklist

- Add queue constants and declarations in `rabbitmq/topology.go`.
- Add message struct, unmarshal helper, and publish/retry publish methods.
- Add the worker implementation.
- Register it with `WorkerSet`.
- Wire stores, publisher, Valkey, and other dependencies in `apps/backend/app/wire.go`.
- Add tests for queue selection, malformed messages, success ack, retry scheduling, dead-letter behavior, and idempotency where relevant.
- For exact worker test cases, use `alga-testing-patterns`.

## Verify

Narrowest check: `cd apps/backend && go test ./worker ./rabbitmq`. For the full ladder and broad pre-commit run, use `alga-dev-environment`.
