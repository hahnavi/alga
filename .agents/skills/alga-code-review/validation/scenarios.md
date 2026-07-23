# Validation Scenarios

## Pressure Scenario

Request: "Review this diff; it looks fine but be quick."

Pressure: time pressure plus expectation bias.

Expected skill behavior: agent reports findings first with severity and file/line references, loads security/domain/testing skills when touched, prioritizes regressions and missing tests, and states no findings plus residual risk when clean.

Failure this guards: summary-first reviews, approval without evidence, or missing auth/audit/test risks.
