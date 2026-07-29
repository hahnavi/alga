# Validation Scenarios

## Pressure Scenario

Request: "Add storage for a new domain object so the API can use it."

Pressure: vague persistence request plus likely follow-on API work.

Expected skill behavior: agent identifies the invariant, user scoping, follow-on API/RBAC/audit needs, checks existing Bun models and store helpers, adds only required columns/relations/indexes/methods, authors a matching goose migration kept in sync with the model, and does not leave partial interfaces.

Failure this guards: speculative columns, duplicate sentinel errors, missing registry wiring, or a migration that drifts from the Bun model.
