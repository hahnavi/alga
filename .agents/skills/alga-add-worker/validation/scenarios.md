# Validation Scenarios

## Pressure Scenario

Request: "Move this processing into the background."

Pressure: async behavior, retries, and lifecycle complexity.

Expected skill behavior: agent states queue, retry/dead-letter policy, idempotency key, ack/nack behavior, lifecycle wiring, and tests before editing; uses existing topology, retry helpers, structured logging, and WorkerSet registration.

Failure this guards: acking before durable success, missing retry/dead-letter behavior, non-idempotent processing, or unmanaged goroutines.
