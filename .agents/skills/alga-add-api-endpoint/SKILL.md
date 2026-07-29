---
name: alga-add-api-endpoint
description: Use when adding or changing Alga REST API handlers, route registration, RBAC gates, store wiring, frontend API methods, router entries, or pages.
priority: P1
tags: [backend, frontend, api, rbac]
---

# Add a REST API Endpoint

Use this for HTTP surface changes. If the endpoint needs a new persisted entity, use `alga-add-db-entity` first. If it touches auth, RBAC, sessions, secrets, user-scoped data, or mutations, also use `alga-security-checklist`.

Before editing, state the route classification, middleware, RBAC permission, mutation audit event, and tests you expect to add. If any item does not apply, say why.

## Check First

- Routes: `apps/backend/api/http.go`.
- Similar handlers: `apps/backend/api/*_handler.go` or domain files under `apps/backend/api/`.
- Helpers: `apps/backend/api/helpers.go` and `apps/backend/api/http.go`.
- Stores: `apps/backend/store/` and `apps/backend/store/registry.go`.
- Frontend API: `apps/frontend/src/lib/api.ts`.
- Frontend routes: `apps/frontend/src/router.ts`.

## Backend Rules

- Classify route before coding: public/callback, authenticated self-scoped, RBAC-protected, or agent bearer.
- Keep handlers thin: validate input, call store/service, audit mutations, return JSON.
- Use `decodeJSON`, `parseLimitSkip`, `writePaginatedJSON`, `writeError`, `writeInternalError`, `writeJSON`, `ensureSlice`, and `pathID`; do not reimplement them.
- Use stores/services instead of raw database access in handlers.
- Constructor-required dependencies do not need nil guards. Setter/optional dependencies must return 503 in every handler that uses them.
- Mutations audit with `s.audit`; SSE broadcasts never replace persistence or audit.
- Use route middleware for auth, CSRF, RBAC, and rate limiting; do not inline replacement logic.

## Route Registration

Prefer Go method routes for new endpoints:

```go
mux.HandleFunc("GET /api/v1/things", s.authMiddleware(s.handleListThings, rbac.ThingRead))
mux.HandleFunc("POST /api/v1/things", s.authMiddleware(s.handleCreateThing, rbac.ThingWrite))
mux.HandleFunc("GET /api/v1/things/{id}", s.authMiddleware(s.handleGetThing, rbac.ThingRead))
mux.HandleFunc("PATCH /api/v1/things/{id}", s.authMiddleware(s.handlePatchThing, rbac.ThingWrite))
mux.HandleFunc("DELETE /api/v1/things/{id}", s.authMiddleware(s.handleDeleteThing, rbac.ThingDelete))
```

If extending an existing prefix dispatcher, keep the local style and check permissions inside each method branch with `s.checkPermission`.

## Handler Shape

```go
func (s *Server) handleCreateThing(w http.ResponseWriter, r *http.Request) {
    if s.thingStore == nil {
        writeError(w, http.StatusServiceUnavailable, "thing store not configured")
        return
    }

    var req struct {
        Name string `json:"name"`
    }
    if !decodeJSON(w, r, &req) {
        return
    }
    name := strings.TrimSpace(req.Name)
    if name == "" {
        writeError(w, http.StatusBadRequest, "name is required")
        return
    }

    created, err := s.thingStore.CreateThing(r.Context(), &store.ThingRecord{Name: name})
    if err != nil {
        writeInternalError(w, err, "failed to create thing")
        return
    }
    s.audit(r, store.AuditThingCreated, map[string]any{"thing_id": created.ID.String()})
    writeJSON(w, http.StatusCreated, created)
}
```

Adapt names to the domain. Do not copy this as a complete feature; implement only the route behavior required by the task and tests for that behavior.

## Wiring

- Add `Server` fields and setters in `apps/backend/api/http.go` for optional stores.
- Wire setters in `apps/backend/app/wire.go` after `api.NewServer`.
- Prefer constructor args only for core dependencies already treated as mandatory.
- Add RBAC permissions in `apps/backend/rbac/permissions.go` and role grants in `apps/backend/rbac/roles.go`.
- Add audit constants in `apps/backend/store/audit.go` for mutations.

## Frontend Integration

- Add types and methods in `apps/frontend/src/lib/api.ts` before pages call them.
- **Always** pass an explicit type parameter to `request<T>(...)` — untyped calls resolve to `Promise<unknown>`.
- **Always** use `e()` for path segments and `buildQuery()` for query strings. Do not inline `encodeURIComponent(...)` or hand-rolled `new URLSearchParams()`.
- Do not add redundant `: Promise<X>` return annotations on non-`async` methods — inference from `request<T>` is the convention.
- Do not add `& { field: ... }` intersections if the base row type already declares the field.
- Place new methods under the matching `// Section Name` comment in `api.ts`.
- Add lazy routes in `apps/frontend/src/router.ts` with `meta.requiredPermission` matching backend RBAC: `const XxxPage = () => import("@/pages/XxxPage.vue");`. Never eager-import pages.
- If the new route needs a document/sidebar title, extend `EXACT_TITLES` or `PREFIX_TITLES` in `apps/frontend/src/lib/pageTitles.ts`. Do **not** add a branch to `router.ts`'s `afterEach` or to `App.vue`'s `pageTitle` computed.
- Gate in-page RBAC UI with `useEntityPermissions("<prefix>")` rather than scattering `auth.hasPermission(...)` string literals.
- Wire destructive endpoints through `useDelete<T>()` + `ConfirmDialog`; render one-time secrets/tokens with `SecretDisplay`.
- Build pages with existing UI primitives and composables; never call `fetch()` from Vue files.

## Tests and Verification

- Backend handler tests should cover unauthorized, forbidden, invalid input, missing dependency (503), success, and mutation audit.
- Frontend checks should cover type safety and route/page integration when changed.
- For detailed test patterns use `alga-testing-patterns`; for the full command ladder use `alga-dev-environment`.

Narrowest backend check: `cd apps/backend && go test ./api ./store`. Frontend: `pnpm --filter frontend typecheck && pnpm --filter frontend lint`.
