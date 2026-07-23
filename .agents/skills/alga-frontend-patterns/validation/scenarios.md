# Validation Scenarios

## Pressure Scenario

Request: "Add a page that calls the new API."

Pressure: page-first implementation and custom UI temptation.

Expected skill behavior: agent identifies API method, route permission, reusable primitive/composable, loading/error/empty states, and responsive/accessibility behavior before editing; adds typed API methods in `api.ts` before page calls; imports icons from `@lucide/vue`.

Failure this guards: `fetch()` in components, duplicate UI controls, missing route permissions, or inaccessible error states.
