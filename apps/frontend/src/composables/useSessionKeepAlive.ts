import { onBeforeUnmount, watch } from "vue";
import { useAuthStore } from "@/stores/auth";

/** Default: refresh every 10 minutes while the app is open (session default is 24h). */
const DEFAULT_INTERVAL_MS = 10 * 60 * 1000;
const MIN_INTERVAL_MS = 60 * 1000;

function refreshIntervalMs(): number {
  const raw = import.meta.env.VITE_SESSION_REFRESH_INTERVAL_MS as string | undefined;
  if (raw != null && raw !== "") {
    const n = Number(raw);
    if (Number.isFinite(n) && n >= MIN_INTERVAL_MS) {
      return n;
    }
  }
  return DEFAULT_INTERVAL_MS;
}

/**
 * Extends the HttpOnly session cookie on a timer and when the tab becomes visible again,
 * so active users are not logged out at the absolute session wall time.
 */
export function useSessionKeepAlive() {
  const auth = useAuthStore();
  let intervalId: number | null = null;
  let inFlight: Promise<boolean> | null = null;
  let lastTouchAt = 0;

  async function touchSession(force?: boolean): Promise<boolean> {
    if (!auth.user) {
      return false;
    }
    if (!force && Date.now() - lastTouchAt < MIN_INTERVAL_MS) {
      return false;
    }
    if (inFlight) {
      return inFlight;
    }
    lastTouchAt = Date.now();
    inFlight = auth.refreshSession().finally(() => {
      inFlight = null;
    });
    return inFlight;
  }

  function onVisibilityChange() {
    if (document.visibilityState !== "visible" || !auth.user) {
      return;
    }
    void touchSession();
  }

  function start() {
    stop();
    const ms = refreshIntervalMs();
    intervalId = window.setInterval(() => {
      void touchSession(true);
    }, ms);
    document.addEventListener("visibilitychange", onVisibilityChange);
  }

  function stop() {
    if (intervalId != null) {
      window.clearInterval(intervalId);
      intervalId = null;
    }
    document.removeEventListener("visibilitychange", onVisibilityChange);
  }

  watch(
    () => auth.user,
    (u) => {
      // No immediate touch here: the session was just created (login) or
      // validated (reload → /auth/me), and a forced refresh would rotate the
      // session ID while the notification/SSE requests that fire on the same
      // auth.user change are still in flight, 401-ing them.
      if (u) {
        start();
      } else {
        stop();
      }
    },
    { immediate: true },
  );

  onBeforeUnmount(stop);
}
