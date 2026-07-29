---
name: alga-testing-patterns
description: Use when adding, changing, or reviewing Alga backend, frontend, store, API, worker, integration, or regression tests.
priority: P1
tags: [testing, backend, frontend, api, store, worker, regression]
---

# Alga Testing Patterns

Use this to choose focused tests and verification commands. Prefer tests that prove the behavior contract, not snapshots of implementation details.

## Check First

- Existing tests near the changed code.
- Backend package layout under `apps/backend`.
- Frontend scripts in `apps/frontend/package.json`.
- Integration package manifests under `integrations/` when touched.

## API Handlers

Cover the route's classification and behavior:

- Unauthorized when the route requires auth.
- Forbidden when RBAC should reject a valid user.
- Invalid input for bad JSON, bad path params, missing required fields, enum/state violations, and ownership failures.
- Missing optional dependency returns 503 when the handler uses setter-injected stores/services.
- Success response status and JSON shape.
- Mutation audit where feasible.
- Rate-limit behavior for auth-adjacent, callback, token, or agent routes when changed.

## Stores

Cover the methods and invariants introduced:

- Create/get/list/update/delete behavior actually added.
- Not-found translation with existing store errors.
- Duplicate key or partial-index behavior when relevant.
- Pagination, filtering, and sort parsing.
- Transaction rollback for multi-write operations.
- Soft-delete vs hard-delete behavior matching the domain.

When the schema changed, apply the goose migration (kept in sync with the Bun model) before testing: `go run . db migrate`.

## Workers and RabbitMQ

Cover message and lifecycle behavior:

- Queue name and prefetch count.
- Malformed message nack behavior.
- Success ack after durable processing.
- Retry count increments and retry publishing.
- Dead-letter behavior after max retries.
- Idempotency key behavior.
- Shutdown or extra goroutine cancellation when introduced.

## Frontend

Use typecheck and lint as baseline. Add component or page tests only when the repo already has a nearby pattern or the change includes complex state transitions.

Check:

- API methods are typed before pages call them.
- Route metadata matches backend RBAC.
- Loading, empty, error, and disabled states are represented.
- Destructive actions use existing confirmation/delete composables.
- User-visible errors use accessible alerts where appropriate.

## Verification Ladder

Use the smallest command that proves the touched behavior, then broaden when shared contracts changed:

```bash
cd apps/backend
go test ./api ./store
```

```bash
cd apps/backend
go run . db migrate
go test ./store
```

```bash
cd apps/backend
go test ./worker ./rabbitmq
```

```bash
pnpm --filter frontend typecheck
pnpm --filter frontend lint
```

Before finishing broad backend or frontend work:

```bash
cd apps/backend
go test ./...
```

```bash
pnpm typecheck
pnpm lint
pnpm build
```
