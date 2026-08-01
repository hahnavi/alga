# AGENTS.md

This is the operating guide for Alga agents. Keep it short, current, and biased toward rules that prevent bad code. Put detailed API behavior, command catalogs, env tables, and endpoint inventories in source files, `.env.example`, tests, package docs, or task-specific skills.

## Skill Routing

- Load the applicable Alga skill before changing code: API, database/models, worker, backend, frontend, integration SDK, security, or dev environment.
- Use `alga-domain-invariants` for alert, incident, investigation, scheduler, escalation, SLA, notification, or lifecycle changes.
- Use `alga-security-checklist` for any change touching routes, auth, RBAC, sessions, secrets, tokens, user-scoped data, integrations, or mutations.
- Use `alga-testing-patterns` for test scope and verification commands.
- Use `alga-dev-environment` for setup, build, test, format, migration, or troubleshooting commands.
- Use `alga-release` for cutting a release — picking the SemVer tag from Conventional Commits, pre-flight checks, release notes, version wiring, and pushing a `v*` tag to trigger `release.yml`.
- Follow nearby code first; if this guide and code disagree, update this guide or remove stale guidance.

## Non-Negotiable Principles

These rules override convenience. Violating them is a bug.

### Avoid Duplication

- Search existing code before adding new helpers, stores, components, composables, routes, or API methods.
- Use HTTP helpers from `apps/backend/api/helpers.go` (`decodeJSON`, `parseLimitSkip`, `writePaginatedJSON`, `ensureSlice`, `pathID`) and `apps/backend/api/http.go` (`writeJSON`, `writeError`, `writeInternalError`).
- Use store helpers from `apps/backend/store/pg_helpers.go` (`pgctx`, `rollbackTx`, `handleQueryErr`, `pgStoreBase`, duplicate-key helpers, limit/skip extraction, sort parsing).
- Put frontend HTTP calls in `apps/frontend/src/lib/api.ts`; do not call `fetch()` from pages or components.
- Reuse frontend composables in `apps/frontend/src/composables/` before creating local async, delete, search, SSE, clipboard, filter, or form logic.
- Reuse UI primitives from `apps/frontend/src/components/ui/`; do not create duplicate Button, Input, Modal, Card, EmptyState, ErrorBanner, or loading components.
- Import frontend icons from `@lucide/vue`, never `lucide-vue-next`.
- Do not use left accent border strips (colored `border-l` rails) anywhere in the UI.
- Reuse existing store errors such as `store.ErrNotFound` and entity-specific not-found errors. Do not create overlapping sentinel errors.
- Use structured `logger.Info/Warn/Error` logging. Do not use `fmt.Println`, `log.Printf`, or ad-hoc debug output.

If code starts looking copy-pasted, stop and extract or reuse a shared helper, composable, primitive, or store method.

### Secure By Default

- Classify every route before implementation: public/callback, authenticated self-scoped, RBAC-protected operator/frontend, or agent bearer.
- All `/api/v1/` frontend/operator routes go through `authMiddleware()` unless intentionally public or callback-only.
- Pass RBAC permissions to `authMiddleware(handler, rbac.Xxx)` when one permission covers the route. Use `s.checkPermission` inside multi-method dispatchers.
- Agent routes use `agentBearerMiddleware(agentRateLimitMiddleware(...))` and do not rely on CSRF cookies.
- Public, callback, auth-adjacent, and token-adjacent routes must be explicit, defensible, and rate limited through existing middleware.
- CSRF is handled by `authMiddleware`; do not build separate CSRF paths.
- Use `decodeJSON()` for request bodies. Never read `r.Body` directly in API handlers.
- Never trust `user_id` in request bodies for authorization. Derive user-scoped access from auth context.
- Store tokens and secrets only as HMAC hashes or encrypted values through existing crypto/store paths. Never persist plaintext secrets.
- Use constant-time comparison for secrets, tokens, signatures, and CSRF values.
- Do not expose secrets, token hashes, peppers, encryption keys, or plaintext credentials in responses or logs. Newly created bearer tokens are shown once.
- Startup must fail closed without required crypto config (`ENCRYPTION_KEYS` or `ENCRYPTION_KEY`, plus `SECRET_PEPPER`) in every environment, not only production. HSTS is emitted on HTTPS regardless of the `SecureCookies` flag.
- Add audit events for every create, update, delete, command, or state transition. Audit logging is fire-and-forget and must not block request success.
- Hard-delete only when the domain already uses hard-delete safely.
- Use Bun query builders and bound parameters; never concatenate values into SQL strings.

### Modern Code Only

- Exact versions live in manifests: backend `apps/backend/go.mod`; frontend `apps/frontend/package.json`; workspace package scripts in root and package `package.json` files.
- Go code targets the module version in `apps/backend/go.mod`. Prefer method-based routing, `errors.Is/As`, `context.WithTimeout`, `sync.WaitGroup`, generics when useful, `range` over integers, and `min/max` builtins.
- TypeScript uses Vue 3 `<script setup lang="ts">`, `import type`, `satisfies`, and `unknown` with narrowing instead of `any`.
- Avoid `interface{}` in Go; use `any` only when a truly unconstrained type is needed.
- Avoid `fmt.Sprintf` for simple string concatenation.
- Avoid `@ts-ignore`; fix types instead.
- Avoid deep optional-chaining as a substitute for proper state modeling.

## Source Of Truth

- Environment variables: root `.env.example`, `apps/backend/.env.example`, and `apps/frontend/.env.example`.
- Backend API routes: `apps/backend/api/http.go`; handler behavior in `apps/backend/api/`; frontend API methods in `apps/frontend/src/lib/api.ts`.
- RBAC permissions and roles: `apps/backend/rbac/`; frontend route metadata in `apps/frontend/src/router.ts`.
- Database schema: Bun models in `apps/backend/db/models/`; SQL migrations in `apps/backend/db/migrations/` (goose); connection pool and migration wiring in `apps/backend/db/client.go` and `apps/backend/db/migrate.go`.
- Stores: `apps/backend/store/` and `apps/backend/store/registry.go`.
- Worker queues and lifecycle: `apps/backend/worker/`, `apps/backend/rabbitmq/`, and app wiring in `apps/backend/app/`.
- Frontend stack, scripts, and dependencies: `apps/frontend/package.json`; shared UI in `apps/frontend/src/components/ui/`; shared behavior in `apps/frontend/src/composables/`.
- Integrations: inspect `integrations/` in the current checkout. Do not assume SDK directories exist.

## Commands

Prefer the smallest relevant verification command, then broaden before commits or when shared behavior changes. See `alga-dev-environment` for the full catalog (dev servers, build/test/format/vet, verification ladder, CLI, migrations). Source-of-truth manifests: `.moon/tasks.yml`, `apps/backend/go.mod`, root and `apps/frontend/package.json`.

Schema changes are hand-written: add or edit the Bun model in `apps/backend/db/models/`, then author a matching goose SQL migration in `apps/backend/db/migrations/`. There is no code-generation step.

## Critical Domain Invariants

Alert, incident, investigation, scheduler, escalation, SLA, and notification behavior is load-bearing and easy to break. Load `alga-domain-invariants` before changing any of it. The short version: `alert_number` is the unique alert ID (fingerprints are dedup keys), one open alert per fingerprint via partial unique index, resolved alerts are never auto-reopened, incidents follow `detected → triaging → active → mitigated → resolved → closed`, and the investigation scheduler binds pending work atomically to online agents.

## Documentation Hygiene

- Keep this file compact; prefer links and source-of-truth pointers over duplicated catalogs.
- Do not paste complete endpoint inventories, environment tables, dependency lists, or implementation recipes here.
- If code and this guide disagree, update this guide or remove the stale guidance.

## Forbidden Practices

Crisp "never do X" rules for both humans and AI agents. Violating any of these is a bug.

- No placeholder or stub implementations: never ship `// TODO` as the actual implementation, and never ship fake/mock bodies in non-test code.
- No ignoring returned errors: do not discard `err` from a function that can fail; handle it or wrap it (`fmt.Errorf("...: %w", err)`).
- No bypassing authorization, RBAC, or ownership checks: every access must go through the established auth/permission gates.
- No embedding secrets: never put tokens, passwords, API keys, or credentials in source or committed config; use the secret store and env injection.
- No hidden global mutable state: avoid package-level mutable vars that escape controlled initialization; pass dependencies explicitly.
- No `fmt.Println`/`log.Printf`/ad-hoc debug output: use structured `logger.Info/Warn/Error` calls instead.

## Decision Priority

When requirements conflict, resolve in this order: **Correctness → Simplicity → Maintainability → Security → Performance.** Earlier priorities win over later ones, unless satisfying an earlier priority would open a security or correctness hole (in which case the hole must be closed first).
