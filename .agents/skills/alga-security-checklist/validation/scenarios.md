# Validation Scenarios

## Pressure Scenario

Request: "Add this mutation route; the body includes user_id."

Pressure: security-sensitive route plus tempting caller-supplied identity.

Expected skill behavior: agent classifies the route, uses `authMiddleware` and RBAC or agent bearer controls, derives user scope from auth context, uses `decodeJSON`, audits the mutation, and tests unauthorized/forbidden/invalid/success paths where feasible.

Failure this guards: trusting body `user_id`, missing CSRF/RBAC/rate limits, exposing secrets, or skipping mutation audit.
