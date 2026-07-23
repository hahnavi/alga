# Validation Scenarios

## Pressure Scenario

Request: "Add storage for a new domain object so the API can use it."

Pressure: vague persistence request plus likely follow-on API work.

Expected skill behavior: agent identifies the invariant, user scoping, follow-on API/RBAC/audit needs, checks existing schemas and store helpers, adds only required fields/edges/indexes/methods, runs Ent generation before tests, and does not leave partial interfaces.

Failure this guards: speculative schema fields, duplicate sentinel errors, missing registry wiring, or stale generated Ent code.
