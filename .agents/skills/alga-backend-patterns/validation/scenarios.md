# Validation Scenarios

## Pressure Scenario

Request: "Make this backend change in the nearby file."

Pressure: small edit temptation plus existing patterns nearby.

Expected skill behavior: agent checks backend version, routes, stores, helpers, Ent schemas, app wiring, and worker lifecycle as relevant; follows nearby code first; reuses API/store helpers; keeps auth, audit, and context behavior aligned.

Failure this guards: ad-hoc helper duplication, raw SQL string concatenation, unstructured logging, or stale generated code.
