import { watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";

/**
 * Owns the two top-level watchers that connect auth state to route
 * navigation. Wires the SPA so:
 *   - logged-out users hitting a non-public route get redirected to
 *     /login
 *   - the initial auth fetch fires when the user lands on a guarded
 *     route while the store is still empty
 *
 * The `auth.loading` check is intentional: toggling loading (e.g. as
 * part of a guestOnly guard's fetch) would re-issue `router.replace`
 * and cancel the in-flight navigation, producing an infinite
 * /auth/me loop. Leaving `loading` out of the dep list keeps both
 * watchers stable while the store is mid-fetch.
 */
export function useAuthBootstrap() {
  const auth = useAuthStore();
  const route = useRoute();
  const router = useRouter();

  watch(
    () => [auth.user, route.path, route.meta.public] as const,
    ([user, path, isPublic]) => {
      if (path === "/login" || isPublic || auth.loading) return;
      if (!user) {
        void router.replace({ path: "/login" });
      }
    },
    { flush: "post", immediate: true },
  );

  watch(
    () => route.path,
    (path) => {
      if (!auth.user && !auth.loading && !route.meta.public && path !== "/login") {
        void auth.fetchCurrentUser();
      }
    },
    { immediate: true },
  );
}
