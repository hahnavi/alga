---
name: alga-backend-patterns
description: Use when writing or modifying Go backend code, stores, Ent schemas, API handlers, app wiring, workers, or backend tests in Alga.
priority: P0
tags: [backend, go, api, store, ent, reference]
---

# Alga Backend Patterns

Backend root: `apps/backend`. Follow nearby code first, then this guide. For new endpoints, entities, or workers, use the dedicated Alga skill in addition to this one.

For alert, investigation, incident, scheduler, RabbitMQ, Valkey, or lifecycle changes, use `alga-domain-invariants` before designing behavior.

## Check First

- Backend version and dependencies: `apps/backend/go.mod`.
- API routes and server wiring: `apps/backend/api/http.go`.
- Stores and helpers: `apps/backend/store/` and `apps/backend/store/pg_helpers.go`.
- Ent schemas: `apps/backend/ent/schema/`.
- App wiring: `apps/backend/app/wire.go`.
- Worker lifecycle: `apps/backend/worker/` and `apps/backend/rabbitmq/`.

## Go Style

- Target the toolchain in `apps/backend/go.mod`. Run `gofmt`; prefer modern idioms over legacy patterns.
- Use three import groups: stdlib, third-party, internal `alga/...`.
- Prefer the standard `slices` and `maps` packages over hand-rolled loops for contain/sort/delete/clone operations.
- Use `range` over integers (`for i := range n`) instead of `for i := 0; i < n; i++`, and the built-in `min`/`max` instead of `if` ladders.
- Prefer type parameters/generics when the logic is genuinely shape-agnostic; existing examples include `ensureSlice[T]`, `handleQueryErr[T]`, and store counter helpers.
- Return errors last and wrap external/DB/network errors with `%w`; surface them via `errors.Is` / `errors.As`.
- Use `context.WithTimeoutCause` (or `context.WithTimeout`) for DB/network/service boundaries unless an existing helper already does; never store contexts in structs.
- Use `any` instead of `interface{}` only when an unconstrained type is necessary.
- Use structured `logger.Info/Warn/Error` with stable key/value pairs; never `fmt.Println`, `log.Printf`, or ad-hoc debug output.
- Prefer `errors.Join` for aggregating multiple failures when relevant.
- Avoid `fmt.Sprintf` for simple string concatenation; prefer `+` or `strings.Builder` for hot paths.

## Resource and Performance

- Outbound HTTP must use `http.NewRequestWithContext` with a deadline-bearing context (or a configured `&http.Client{Timeout: ...}`); never `http.DefaultClient`/`http.PostForm`/bare `http.NewRequest`. Cap response reads with `io.LimitReader`.
- Branch on errors via `errors.Is`/`errors.As` against wrapped sentinels; never `strings.Contains(err.Error(), ...)`. Reuse existing store sentinels (`store.ErrNotFound`, `store.ErrIncidentNotFound`, ...).
- Never do a synchronous DB write on a per-request hot path (token `last_used_at`, counters). Use the throttled async pattern from `personal_access_tokens.go` (`updateLastUsed` with a 24h guard).
- Compile regex once via the cached `matching.GetCompiledRegex`; never `regexp.Compile` inside a loop or per-call path.
- Write SSE events through the shared `sse.WriteEvent` helper; do not rebuild the frame with `string +=` per event.
- Avoid N+1: batch `IN` queries or Ent `.With*()` eager-load instead of a store/HTTP call per loop iteration. Pre-size slices with `make([]T, 0, n)` when the bound is known.
- Fire-and-forget goroutines (`audit.Log`, side-effects) must `recover()` and stay bounded; prefer a semaphore or single consumer over unbounded fan-out.

## Stores

- Store interfaces and records live in `apps/backend/store`.
- PostgreSQL stores embed `pgStoreBase` and are registered in `store/registry.go`.
- Use `pgctx`, `rollbackTx`, `handleQueryErr`, duplicate-key helpers, limit/skip extraction, and sort parsing from `pg_helpers.go`, and `nextPgCounter` from `store.go`.
- Use Ent predicates/builders; do not concatenate SQL strings.
- Use transactions only when multiple writes must be atomic.

## API

- Handlers live in `apps/backend/api` and stay thin: validate input, call store/service, audit mutations, return JSON.
- Use helpers from `api/helpers.go` and `api/http.go`: `decodeJSON`, `parseLimitSkip`, `writePaginatedJSON`, `writeError`, `writeInternalError`, `writeJSON`, `ensureSlice`, and `pathID`. Do not reimplement them or read `r.Body` directly.
- Prefer method routes for new endpoints. Match existing prefix dispatchers when extending one.
- Guard optional stores with 503 in every handler that uses them.
- Map domain failures to the right status: `404` via `handleQueryErr`/`store.ErrNotFound`, `409` for duplicate/invariant conflicts, `422` for validation, `502`/`504` for upstream integration timeouts. Avoid leaking internal details in messages.
- Keep response models explicit and decoupled from persistence records (which may carry sensitive fields). Map records to response types at the handler boundary.
- Add audit events for mutations and state transitions.
- Publish SSE only when the relevant publisher is configured.

## Auth, RBAC, CSRF

Baseline route classification, middleware, CSRF, and rate-limit rules live in AGENTS.md "Secure By Default" (always in context) and are expanded by `alga-security-checklist`; don't re-derive them here. Backend specifics: classify each route before implementing, call `s.checkPermission` inside every multi-method dispatcher branch, and guard every public/callback route with existing rate-limit middleware.

## Ent

- Schemas live in `apps/backend/ent/schema`.
- Use project helpers such as UUID IDs and `timeNow` timestamps.
- Run Ent generation after schema edits and keep generated files in sync.

```bash
cd apps/backend
go generate ./ent
go test ./ent/... ./store
```

## Workers

Worker interface and registration are in `worker/worker.go`. RabbitMQ topology is in `rabbitmq/topology.go`. Use `alga-add-worker` for new consumers or retry flows.

## Verify

Use `alga-testing-patterns` when adding or changing tests. Prefer the smallest relevant package tests while iterating, then broaden when shared behavior changed.

```bash
cd apps/backend
gofmt -w .
go vet ./...
go test ./...
```

`go vet ./...` catches common mistakes (printf misuse, unreachable code, lock copies). Run it alongside tests before committing.
