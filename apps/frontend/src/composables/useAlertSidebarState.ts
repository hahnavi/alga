import { computed, nextTick, onBeforeUnmount, ref } from "vue";
import { useResizableMain } from "@/composables/useResizableMain";
import { useEscapeKey } from "@/composables/useEscapeKey";

/**
 * Owns the alert detail page's right-hand thread drawer state:
 *   - which alert the drawer is open for (so we can close it on route
 *     change without flashing the previous alert's thread)
 *   - the open/close transition (`showThread` + `threadLeaving`)
 *   - the responsive shell flags (`threadLayoutOpen`)
 *   - the resizable main column width
 *   - the Escape-to-close binding
 *
 * The page only needs to read these refs and call `open` / `close` /
 * `toggle`; the composable drives the layout class computed values
 * via the `drawerVisible` and `threadTransitioning` computeds.
 */
export function useAlertSidebarState() {
  const { mainWidth, nudgeMainWidth, resizingMain, startMainResize } = useResizableMain();

  const showThread = ref(false);
  const threadLayoutOpen = ref(false);
  const threadLeaving = ref(false);

  function open() {
    threadLayoutOpen.value = true;
    showThread.value = true;
    void nextTick();
  }

  function close() {
    showThread.value = false;
    threadLeaving.value = true;
  }

  function onAfterLeave() {
    if (!showThread.value) {
      threadLayoutOpen.value = false;
    }
    threadLeaving.value = false;
  }

  function toggle() {
    if (showThread.value) {
      close();
      return;
    }
    open();
  }

  useEscapeKey(close, () => showThread.value);

  // The page uses these in its layout class computeds.
  const drawerVisible = computed(
    () => showThread.value || threadLeaving.value || threadLayoutOpen.value,
  );
  const threadTransitioning = computed(() => showThread.value || threadLeaving.value);

  // Reset drawer state when the page unmounts (route away from the
  // detail view) so a re-entry doesn't briefly show the previous thread.
  onBeforeUnmount(() => {
    showThread.value = false;
    threadLayoutOpen.value = false;
    threadLeaving.value = false;
  });

  return {
    mainWidth,
    nudgeMainWidth,
    resizingMain,
    startMainResize,
    showThread,
    threadLayoutOpen,
    threadLeaving,
    drawerVisible,
    threadTransitioning,
    open,
    close,
    toggle,
    onAfterLeave,
  };
}
