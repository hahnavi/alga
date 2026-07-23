---
name: alga-code-review
description: Use when reviewing Alga diffs, pull requests, implementation branches, or generated changes for correctness, regressions, security, tests, and maintainability.
priority: P1
tags: [review, code-review, backend, frontend, security, tests]
---

# Alga Code Review

Review as a production code reviewer. Findings first, ordered by severity, with file and line references.

## Load With

- `alga-security-checklist` for routes, auth, RBAC, sessions, secrets, tokens, integrations, user-scoped data, or mutations.
- `alga-domain-invariants` for alerts, incidents, investigations, scheduler, RabbitMQ, Valkey, escalation, or lifecycle behavior.
- `alga-testing-patterns` for missing or weak tests.
- Backend/frontend/API/entity/worker skills when the diff touches those areas.

## Review Focus

- Behavioral regressions and broken contracts.
- Auth, RBAC, CSRF, rate limits, token handling, and user scoping.
- Missing audit events for create, update, delete, command, and state transitions.
- Store/API/frontend type drift.
- Duplicated helpers, API calls outside `api.ts`, duplicate UI primitives, or ad-hoc async logic.
- Ent schema, generated code, migrations, partial indexes, and transaction safety.
- Worker ack/nack, retry, dead-letter, idempotency, and lifecycle behavior.
- Frontend loading, empty, error, disabled, responsive, and accessibility states.
- Missing tests that would catch the risk.

### Frontend consistency red flags

- Untyped `request(...)` calls in `api.ts` (resolve to `Promise<unknown>`).
- Raw `encodeURIComponent(...)` in path templates or hand-rolled `new URLSearchParams()` + `` ?${q} `` instead of the `e()` and `buildQuery()` helpers.
- Redundant `: Promise<X>` annotations on non-`async` API methods.
- Redundant `& { token: string }`-style intersections when the base row type already declares the field.
- Eager `import X from "@/pages/X.vue"` in `router.ts` (all routes should be lazy `() => import(...)`).
- New `if (path === ...)` branches in `router.ts` `afterEach` or `App.vue` `pageTitle` instead of extending `lib/pageTitles.ts`.
- Scattered `auth.hasPermission("xxx:write")` string literals where `useEntityPermissions("xxx")` computeds belong.
- Bespoke confirm/delete modals instead of `useDelete<T>()` + `ConfirmDialog`.
- Custom secret/token reveal+copy UI instead of `SecretDisplay`.
- Hand-rolled skeleton/placeholder rows instead of `SkeletonRows`; bespoke selects/switches/inputs instead of `Select`/`Switch`/`Input`/`Textarea`/`SearchInput`/`SortSelect`.
- Popover/menu components not using `useDropdownLifecycle` (outside-click + Escape), or custom modals not using `useModalFocusTrap`.
- Direct `@/assets/*-agent-32x32.png` or `@/assets/*-32x32.png` imports instead of `lib/agentAvatar.ts` / `lib/providerIcon.ts`.
- Inline `new Date(iso).toLocaleString()` / `.toLocaleDateString()` instead of `lib/time.ts` helpers.
- Inline `err instanceof Error ? err.message : ...` instead of `getErrorMessage(err, fallback)`.
- Hand-rolled `<input>`, `<textarea>`, `<button>`, error banners, loading spinners, empty states, or modals duplicating `Input`, `Textarea`, `Button`, `ErrorBanner`, `LoadingSpinner`, `EmptyState`, or `Modal`.
- Hand-rolled password validation, user display names, or `?redirect=` parsing instead of `lib/validators.ts`, `lib/userDisplay.ts`, or `lib/redirect.ts`.
- Inline `document.querySelector("main.min-h-0")` or duplicated scroll/theme/notification-click logic instead of the shared `lib/` helpers.
- Call-signature `defineEmits<{ (e: "x", v: T): void }>()` instead of tuple syntax `"x": [T]`; runtime `defineProps({...})` objects instead of `defineProps<{ ... }>()`.
- `const toast = useToast()` or aliased destructures instead of `const { push } = useToast()`.
- `interface` and `type` arbitrarily mixed for plain object shapes within the same file.
- Missing `onBeforeUnmount(() => clearPageHeader())` after `setPageHeader(...)`.

### Backend consistency red flags

- Reading `r.Body` directly instead of `decodeJSON`; missing `pathID`/`parseLimitSkip` reuse.
- Missing 503 guard on an optional (setter-injected) store/service used by a handler.
- Status code drift: `500` for not-found/duplicate (should be `404`/`409`), or leaking internal error text in messages.
- Returning persistence records that embed secrets/token hashes instead of explicit response models.
- Raw SQL string concatenation instead of Ent predicates/builders.
- Missing audit on a create/update/delete/command/state-transition handler.
- Duplicated not-found sentinel errors overlapping `store.ErrNotFound`.
- Legacy Go: `interface{}`, `for i := 0; i < n; i++` where `range` fits, manual min/max, hand-rolled slice contain/sort where `slices.*` fits, `fmt.Println`/`log.Printf` debug output.
- Missing `rollbackTx` on a multi-write transaction, or context stored in a struct.

### Backend resource & performance red flags

- Outbound HTTP (`http.Get/Post/PostForm`, `http.DefaultClient`, `http.NewRequest` without context) lacking both a context deadline and a client/request timeout; unbounded `io.ReadAll(resp.Body)` without `io.LimitReader`.
- `http.DefaultClient` (Timeout=0) used directly for outbound calls instead of a configured `&http.Client{Timeout: ...}`.
- Error branching by `strings.Contains(err.Error(), "not found")` (or any error-text matching) instead of `errors.Is`/`errors.As` against a wrapped sentinel.
- A synchronous DB write on every auth/request hot path (e.g. updating `last_used_at` on each token validation) instead of the throttled async pattern used by `personal_access_tokens`.
- `regexp.Compile`/`MustCompile` recompiled per call inside a loop or hot path instead of the cached `matching.GetCompiledRegex`.
- SSE event framing hand-rolled with `string += fmt.Sprintf(...)` instead of the shared `sse.WriteEvent`.
- N+1: a store/HTTP call (`GetByID`, `GetByFingerprint`, `.Query()`, `.Exist()`) executed inside a `for range` over results where a batch `IN` query or Ent `.With*()` eager-load fits; unsized slices (`var s []T`) grown inside a known-length loop.
- `context.Background()`/`context.TODO()` used inside an HTTP handler path where `r.Context()` (with a timeout) should propagate; `go func(){...}()` started without a `recover()` or shutdown WaitGroup.
- Goroutine fan-out with no concurrency bound (e.g. one goroutine per audit event with no semaphore/channel).

## Output Shape

- Start with findings. Include severity and location.
- Include open questions only after findings.
- Include a short summary only after issues.
- If no issues are found, say that clearly and mention residual risk or unrun checks.

## Local Checks

Use narrow commands while reviewing. Prefer reading the changed files and nearby tests before broad builds.

```bash
git diff --stat
git diff --name-only
git diff
```

Then choose verification from `alga-testing-patterns` based on touched code.
