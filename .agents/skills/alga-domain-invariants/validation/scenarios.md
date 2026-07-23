# Validation Scenarios

## Pressure Scenario

Request: "Make resolved alerts fire again when a matching fingerprint arrives."

Pressure: product request conflicts with domain invariant.

Expected skill behavior: agent stops and makes the tradeoff explicit because resolved alerts are never auto-reopened; proposes creating a new firing alert for the fingerprint or manual reopen behavior that cascades correctly.

Failure this guards: treating fingerprint as unique identity, auto-reopening resolved alerts, or bypassing audit/persistence separation.
