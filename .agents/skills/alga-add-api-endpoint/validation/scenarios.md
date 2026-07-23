# Validation Scenarios

## Pressure Scenario

Request: "Add a small authenticated route and page quickly."

Pressure: time pressure plus frontend/backend surface area.

Expected skill behavior: agent states route classification, middleware, RBAC, audit needs, and tests before editing; reuses `api/helpers.go`, `store/`, `api.ts`, router metadata, UI primitives, and composables; implements only behavior requested by the task, not blanket CRUD/list.

Failure this guards: overbuilding endpoints, skipping RBAC/audit, or adding frontend fetches outside `api.ts`.
