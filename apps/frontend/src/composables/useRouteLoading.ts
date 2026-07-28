import { computed, onScopeDispose, ref, type ComputedRef } from "vue";
import type { Router } from "vue-router";

const MIN_VISIBLE_MS = 200;
const COMPLETE_HOLD_MS = 180;

type RouteLoadingTracker = {
  start: () => void;
  finish: () => void;
  fail: () => void;
  dispose: () => void;
  loading: ComputedRef<boolean>;
  completing: ComputedRef<boolean>;
};

function nowMs(): number {
  if (typeof performance !== "undefined") return performance.now();
  return Date.now();
}

export function createRouteLoadingTracker(): RouteLoadingTracker {
  const pending = ref(0);
  const visible = ref(false);
  const completing = ref(false);
  const startedAt = ref(0);
  let hideTimer: number | null = null;
  let completeTimer: number | null = null;

  function clearTimer(which: "hide" | "complete") {
    if (which === "hide" && hideTimer !== null) {
      clearTimeout(hideTimer);
      hideTimer = null;
    } else if (which === "complete" && completeTimer !== null) {
      clearTimeout(completeTimer);
      completeTimer = null;
    }
  }

  function start() {
    pending.value += 1;
    if (pending.value !== 1) return;
    clearTimer("hide");
    clearTimer("complete");
    completing.value = false;
    startedAt.value = nowMs();
    visible.value = true;
  }

  function finish() {
    if (pending.value <= 0) return;
    pending.value = Math.max(0, pending.value - 1);
    if (pending.value > 0) return;
    scheduleHide();
  }

  function fail() {
    pending.value = 0;
    clearTimer("hide");
    clearTimer("complete");
    completing.value = false;
    visible.value = false;
  }

  function scheduleHide() {
    clearTimer("complete");
    completing.value = true;
    completeTimer = setTimeout(() => {
      completing.value = false;
      completeTimer = null;
      const elapsed = nowMs() - startedAt.value;
      const delay = Math.max(0, MIN_VISIBLE_MS - elapsed);
      hideTimer = setTimeout(() => {
        visible.value = false;
        hideTimer = null;
      }, delay);
    }, COMPLETE_HOLD_MS);
  }

  function dispose() {
    clearTimer("hide");
    clearTimer("complete");
  }

  return {
    start,
    finish,
    fail,
    dispose,
    loading: computed(() => visible.value),
    completing: computed(() => completing.value),
  };
}

export function useRouteLoading(router: Router): RouteLoadingTracker {
  const tracker = createRouteLoadingTracker();
  const removeBefore = router.beforeEach(() => {
    tracker.start();
    return true;
  });
  const removeAfter = router.afterEach(() => {
    tracker.finish();
  });
  const removeError = router.onError(() => {
    tracker.fail();
  });
  onScopeDispose(() => {
    removeBefore();
    removeAfter();
    removeError();
    tracker.dispose();
  });
  return tracker;
}
