/**
 * Single source of truth for redirecting an unauthenticated user back to
 * `/login`. Used by the request pipeline (`lib/api.ts`) and the SSE auth
 * probe (`composables/useSSE.ts`).
 *
 * `router.replace` preserves Pinia state and in-flight requests, unlike a
 * hard `window.location.href` navigation. We fall back to the hard redirect
 * when the router is unavailable (e.g. very early in app boot, or in test
 * environments where no app is mounted).
 */
import { useRouter } from "vue-router";

let cachedRouter: ReturnType<typeof useRouter> | null = null;

/** Set the router instance once, during app boot (see `App.vue`). */
export function setAuthRedirectRouter(router: ReturnType<typeof useRouter>) {
  cachedRouter = router;
}

export function redirectToLogin(): void {
  const current = encodeURIComponent(window.location.pathname + window.location.search);
  const target = `/login?redirect=${current}`;
  if (cachedRouter) {
    void cachedRouter.replace(target);
    return;
  }
  window.location.href = target;
}
