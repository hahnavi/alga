---
name: alga-frontend-patterns
description: Use when writing or modifying Vue pages, components, frontend API methods, route guards, stores, or Tailwind UI in Alga.
priority: P1
tags: [frontend, vue, typescript, tailwind, reference]
---

# Alga Frontend Patterns

Frontend root: `apps/frontend`. Exact versions and scripts live in `apps/frontend/package.json`. Preserve the existing Alga design system unless the user explicitly asks for a redesign.

Before editing a page or component, identify the API method, route permission, shared primitive/composable to reuse, loading/error/empty states, and responsive/accessibility behavior.

## Check First

- API client: `apps/frontend/src/lib/api.ts`.
- Router and permission metadata: `apps/frontend/src/router.ts`.
- UI primitives: `apps/frontend/src/components/ui/`.
- Shared behavior: `apps/frontend/src/composables/`.
- Existing pages: `apps/frontend/src/pages/`.
- Stores: `apps/frontend/src/stores/`.
- Shared helpers: `apps/frontend/src/lib/` (time, pageHeader, pageTitles, agentAvatar, providerIcon, error, toast, storage, redirect, validators, userDisplay, investigation, ownerThread, routeConditions, notificationClick, theme, dialogFocus, routing, scrollContainer, uiClasses).

## Hard Rules

- Use Vue 3 Composition API with `<script setup lang="ts">`; do not use Options API.
- Use `import type` for type-only imports and `satisfies` for literal-shape contracts (sort options, status enums, role maps).
- Do not use TypeScript `any`; use `unknown` and narrow. Prefer inline type narrowing over deep optional-chaining that hides unclear state.
- Prefer `type` aliases for object shapes; do not arbitrarily mix `interface` and `type` for the same shape within a file.
- Import icons from `@lucide/vue`, not `lucide-vue-next`.
- All HTTP calls go through `api` in `@/lib/api.ts`; never call `fetch()` in pages or components (composables included — the only intentional exception is the SSE auth probe in `useSSE`).
- Use `buildQuery()` (re-exported from `@/lib/api`) for query strings. Do not hand-roll `new URLSearchParams()` + `` ?${q} ``. Use `e()` for dynamic path segments. Both helpers are exposed from `@/lib/api`.
- Use shared UI primitives from `@/components/ui/` before writing custom controls. Do not inline `<input>`, `<textarea>`, `<button>`, error banners, loading spinners, empty states, skeletons, selects, switches, avatars, cards, or modals that duplicate the primitives listed below.
- Use existing composables before writing inline submit/delete/search/SSE/filter/clipboard/permission/dropdown/scroll/theme logic.
- Use `defineEmits` tuple syntax: `"event": [payload]` or `"event": []` — not the call-signature `(e: "event", value: T): void` form. Use `defineProps<{ ... }>()` (with `withDefaults` when needed), never `defineProps({...})` runtime objects.
- `defineOptions({ name: "..." })` must be the first statement after imports in every page.
- Access toast via `const { push } = useToast();` — never `const toast = useToast()` or aliased destructures like `const { push: pushToast } = useToast()`.
- Gate RBAC-conditioned UI with `useEntityPermissions("<prefix>")` rather than scattering `auth.hasPermission("xxx:write")` string literals across a page. The composable exposes `canRead`/`canWrite`/`canDelete`/`canCommand` and `can(...)` for multi-perm gating. The only legitimate direct `auth.hasPermission` calls live in nav (`App.vue`, `Sidebar.vue`, `MobileAgentMenu.vue`, `MobileMoreMenu.vue`) and inside `useEntityPermissions` itself.
- Use `usePageHeaderActions({...})` for every list page (search, add, filter buttons). The composable already renders the inline search input via `data-page-header-search`, plus Search/Add/Filter buttons. Reach for raw `setPageHeader`/`clearPageHeader` only on detail pages with bespoke headers, and **always** pair with `onBeforeUnmount(() => clearPageHeader())`.
- Wrap destructive actions with `useDelete<T>()` + `ConfirmDialog` (or the `ConfirmDialog` directly for non-list deletions); never build a bespoke confirm modal.
- Display one-time secrets/tokens with `SecretDisplay`; never roll a custom reveal/copy control.
- Keep pages responsive and accessible with labels, focus-safe dialogs, loading states, empty states, and `role="alert"` for actionable errors (via `ErrorBanner`).
- Use shared time/storage/title/agentAvatar helpers (see below) instead of inlining `new Date(...).toLocaleString`, `localStorage.*`, document titles, or asset imports.
- Use `useListFilter(source, fields, query)` for local search filtering of any list — do not re-implement `q.toLowerCase().trim()` + `list.filter(...)` per page.
- Gate `api.getUsers()` calls with `useEntityPermissions("users").canRead` (or `useUsersIfPermitted`) instead of hand-checking `auth.hasPermission("users:manage")`.
- Use `getAgentAvatarSrc(agentType)` for agent avatars, `getProviderIconSrc("mattermost" | "slack")` for notification icons, and `getAgentBrandIconSrc("hermes" | "openclaw" | "google_meet")` for brand icons. Never import `@/assets/*-32x32.png` directly.

## API Client

- Add exported payload/record types next to related methods in `apps/frontend/src/lib/api.ts`.
- **Always** pass an explicit type parameter to `request<T>(...)`. Untyped calls resolve to `Promise<unknown>` and silently disable caller type-checking.
- Do not add redundant `: Promise<X>` return annotations on non-`async` methods — `request<T>` already returns `Promise<T>` and TS infers it. The exception is `async` methods where the annotation clarifies the narrowed return.
- Do not add intersections like `& { token: string }` if the base row type already declares the field. Check the type first.
- Reuse the existing row/response type when the backend returns the same shape — don't redeclare.
- Group methods under `// Section Name` comments; place methods under the correct section.
- Model backend responses explicitly; do not rely on deep optional chaining to hide unclear state.
- Add frontend methods before pages/components call new endpoints.
- Keep `safe401Paths` in `lib/api.ts` in sync with public callback routes. When you add a `/api/v1/auth/...` public callback that can return 401 to an unauthenticated user (oidc authorize, forgot-password, reset-password), add it to `safe401Paths` to prevent the redirect-loop to `/login` that the inline comment warns about.

Example shape:

```ts
export type XxxRecord = {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
};

getXxxs(params?: { q?: string; limit?: number; skip?: number }) {
  return request<{ items: XxxRecord[]; total: number }>(
    `/api/v1/xxxs${buildQuery({ q: params?.q, limit: params?.limit, skip: params?.skip })}`,
  );
},
```

## Composables and UI

Reuse these before writing inline logic — they already handle race safety, error toast, and cleanup.

Data and forms:
- `useAsyncData<T>()` — one-shot fetch + loading/error state with race-safe reloads.
- `useListPage<T>()` — paginated list pages (items/total/loading/error + race-safe reload).
- `useListFilter(source, fields, query)` — local search filter for any list. Use it instead of hand-rolled per-page `filteredX` computeds.
- `useFormSubmit()` — submit state, toast, and form-error handling (`withSubmit`).
- `useDelete<T>()` — confirmation-backed delete (pair with `ConfirmDialog`).
- `useEntityPermissions(prefix)` — `canRead`/`canWrite`/`canDelete`/`canCommand` computeds + `can(...)` for multi-perm gating.
- `useUsers()` / `useUsersIfPermitted(perm)` — user list lookup for display resolution (permission-gated variant skips the network when the user lacks the perm).
- `useLoadIntegrations()` — cached integration list for selectors.

Realtime and chat:
- `useSSE()` — server-sent events with shared connection, reconnect/backoff, auth probe, and `onReconnect` resync.
- `useStickToBottom()` — auto-scroll container that lifts when the user scrolls up.
- `useTypingIndicator()` — agent/user typing presence.
- `useChatSearch()` — in-message search state and navigation.
- `useScrollRestore()` — restores scroll position across navigations.

Interaction and layout:
- `useSearchDebounce()` — debounced query ref for filters/search inputs.
- `useFilterSync()` — bidirectional URL ↔ filter state for list pages.
- `useClipboard()` — copy-to-clipboard with toast.
- `useEscapeKey(handler, active)` — global Escape binding scoped to `active`.
- `useDropdownLifecycle(openRef, rootRef)` — outside-pointerdown + Escape close for popovers/menus.
- `useResizableMain()` — persisted resizable split layout.
- `useOAuthPopup()` — OAuth popup flow for account linking.
- `useChartOptions()` — shared ApexCharts option baseline.
- `useSessionKeepAlive()` — periodic session refresh.
- `usePageHeaderActions({...})` — list-page header (title, search input, add, filter buttons). Replaces hand-rolled `syncHeader()`. Auto-pairs `setPageHeader` with `onBeforeUnmount(clearPageHeader)`.

New composables that fetch data should expose `loading`/`error` refs (matching `useAsyncData`/`useListPage`) and report failures via `getErrorMessage` + `useToast().push`, not silent `catch {}`.

## UI Primitives (`@/components/ui/`)

Reach for these before any bespoke control. Each already wires accessibility, focus, and Alga theme tokens.

Layout/feedback:
- `Button` (variants: default/outline/destructive; sizes: sm/md), `Card`, `EmptyState`, `ErrorBanner` (`role="alert"`), `LoadingSpinner`, `SkeletonRows`, `ToastStack`.
- `Modal` and `ConfirmDialog` — both use `useModalFocusTrap` for focus trapping and restore. Prefer `ConfirmDialog` for yes/no destructive flows.
- `SecretDisplay` — one-time token/secret reveal + copy. Never build a custom one.

Form controls:
- `Input`, `Textarea`, `Select`, `Switch`, `SearchInput`, `SortSelect`, `TimezoneSelect`, `FormLabel`, `ConditionEditor`, `MarkdownEditor` + `MarkdownRenderer`.

Data display:
- `Avatar`, `UserLabel`, `SeverityBadge`, `ServiceStatusBadge`, `DeletedBadge`, `KeyValueDisplay`, `InteractiveCard`.
- Domain action menus: `AlertActionsMenu`, `ServiceActionsMenu`, `SettingsDialog`, plus `ChatMessageRow`/`ChatEditorBar`/`ChatSearchBar`/`ChatTypingIndicator`/`TypingIndicator`. Verify primitive prop names before assuming an API.

## Pages and Router

- Keep page-level loading in `src/pages/*`; extract reusable behavior to composables.
- **All** routes use lazy imports: `const XxxPage = () => import("@/pages/XxxPage.vue");`. Do not eager-import pages in `router.ts` — it breaks code-splitting and is inconsistent with the rest of the table.
- Gate protected pages with `meta.requiredPermission` matching backend RBAC strings.
- Inside pages, prefer `useEntityPermissions(prefix)` computeds over repeated `auth.hasPermission("xxx:write")` literals. Fall back to `useAuthStore().hasPermission(...)` only for one-off checks outside a prefix (e.g. PAT permission-list filters that aren't prefix-shaped).
- Use `usePageHeaderActions({...})` for list pages. Use `setPageHeader(title, badges?, options?)` + `onBeforeUnmount(() => clearPageHeader())` for detail pages.
- Document titles come from `lib/pageTitles.ts` — do **not** add a new branch to `router.ts`'s `afterEach` or re-derive a title in `App.vue`. Extend `EXACT_TITLES` / `PREFIX_TITLES` in `lib/pageTitles.ts` instead; both `router.ts` and `App.vue` consume `pageTitleForPath(path)`. Per-page overrides go through `routePageTitle` ref (set in setup, cleared in `afterEach`).

## Shared Helpers (lib/)

- `lib/time.ts`: `formatTime`, `formatDate`, `formatTimeOnly`, `formatTimeFull`, `formatTimeAgo`, `formatDateSeparator`, `dateSeparatorKey` (stable grouping key for day separators in chat/thread lists), `formatDurationFromMs`, `formatExpires`, `localDatetimeToRFC3339`. All guard invalid/pre-1970 input and return `—`.
- `lib/pageTitles.ts`: `pageTitleForPath(path)` — single source of truth for path → document/sidebar title.
- `lib/pageHeader.ts`: `setPageHeader(title, badges?, options?)`, `clearPageHeader()`, `createSearchActionButton`, `createAddButton`, `headerSearchState`. Pair `setPageHeader` with `onBeforeUnmount(() => clearPageHeader())` when you reach for it directly.
- `lib/agentAvatar.ts`: `getAgentAvatarSrc(agentType)` and `getAgentBrandIconSrc(brand)` (`"hermes" | "openclaw" | "google_meet" | "other"`; returns `null` for `"other"`). Never import `@/assets/*-agent-32x32.png` or `@/assets/*-meet-32x32.png` directly.
- `lib/providerIcon.ts`: `getProviderIconSrc(provider)` for `"mattermost" | "slack"` notification icons.
- `lib/error.ts`: `getErrorMessage(err, fallback)` — use for every catch; do not write `err instanceof Error ? err.message : ...` or `(err as Error).message` inline.
- `lib/toast.ts`: `useToast()` returning `{ toasts, push, dismiss }`. Always destructure `push`.
- `lib/storage.ts`: `safeGetItem` / `safeSetItem` for `localStorage` (silently handle private-mode/quota failures). Never call `localStorage.*` directly.
- `lib/redirect.ts`: `safeRedirectTarget(queryValue, fallback)` — validated same-origin redirect target (use for `?redirect=` flows).
- `lib/validators.ts`: `validatePassword(password)` — returns `{ valid, error, checks }` for the password policy.
- `lib/userDisplay.ts`: `resolveDisplayName({ userId, username, users, role, agentName, fallback })` — consistent user/agent name resolution. Prefer it over hand-rolling name fallbacks.
- `lib/investigation.ts`: `investigationDisplayId(inv)` — `INV-`/`AINV-` numbering with UUID fallback.
- `lib/ownerThread.ts`: `normalizeOwnerThreadResponse(...)` — coerce wire shape (messages vs items) into the canonical type.
- `lib/routeConditions.ts`: `summarizeCondition(...)`, `CONDITION_SOURCE_OPTIONS`, `CONDITION_OPERATOR_OPTIONS` for route selector editors.
- `lib/notificationClick.ts`: `handleNotificationClick(...)` — mark-read + safe navigation for notification toasts.
- `lib/theme.ts`: `useTheme()` — `isDark`/`mode`/`setMode`/`toggle` with system preference + persistence.
- `lib/dialogFocus.ts`: `useModalFocusTrap(openRef, getContainer, options?)` — focus trap, initial-focus, and restore-on-close for any custom modal. `Modal` and `ConfirmDialog` already use it; use it for any new dialog.
- `lib/routing.ts`, `lib/scrollContainer.ts` (`getScrollContainer()`): shared navigation and scroll helpers.
- `lib/uiClasses.ts`: `HEADER_ICON_BTN_CLASS`, `POPOVER_MENU_*`, `AGENT_ACTION_MENU_*`, `ACCOUNT_MENU_ITEM_CLASS`, `MOBILE_MORE_USER_ACTION_CLASS`. Import the constant instead of inlining the same tailwind string; extend this file (not each page) when adding a shared class (e.g. severity/priority rail colors).

## Modernization Reminders

- Avoid `as unknown as`; use the actual type (`Editor` from `@tiptap/core`, `Partial<T>` with explicit assignments). Prefer `ref`/`shallowRef` over `reactive({...})` for objects that get fully replaced (lists, chart data).
- `as const` for literal arrays/objects is the default; `satisfies` is preferred when the literal has a known shape contract. Vue 3.4+ `defineModel()`/`useId()` are fine when touching the relevant code, but don't migrate proactively.
- Empty `catch {}` is acceptable for intentional cleanup/non-essential paths (add `// intentional`); all other catches funnel through `getErrorMessage` + toast.

## Visual Design

- Preserve established Alga spacing, typography, primitives, and Tailwind v4 style unless asked to change direction. For major redesigns or new visual language, use `ui-ux-pro-max` for exploration, then implement with these Alga rules.

## Verify

Use `alga-testing-patterns` when frontend test scope is unclear.

```bash
pnpm --filter frontend typecheck
pnpm --filter frontend lint
pnpm --filter frontend build
```

Run the smallest relevant subset while iterating. Always run `pnpm --filter frontend typecheck` before claiming a frontend refactor complete — even small composable changes ripple through many pages.
