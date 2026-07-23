# Validation Scenarios

## Pressure Scenario

Request: "What tests are enough for this change?"

Pressure: desire for minimal verification with unknown risk.

Expected skill behavior: agent checks nearby tests and package manifests, selects focused contract tests for touched API/store/worker/frontend behavior, then broadens only when shared contracts changed.

Failure this guards: snapshotting implementation details, skipping unauthorized/forbidden paths, missing store invariants, or running unrelated broad checks as a substitute for focused tests.
