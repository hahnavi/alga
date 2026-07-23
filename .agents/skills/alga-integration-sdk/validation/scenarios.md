# Validation Scenarios

## Pressure Scenario

Request: "Update the SDKs for this agent endpoint."

Pressure: stale assumptions about integration directories and cross-language support.

Expected skill behavior: agent inspects `integrations/` in the current checkout before planning, derives endpoint behavior from `api/http.go` and agent handlers, preserves bearer-token security, and only updates SDK packages that exist or are explicitly requested.

Failure this guards: creating nonexistent SDK directories, trusting copied endpoint inventories, or weakening agent bearer/auth/rate-limit behavior.
