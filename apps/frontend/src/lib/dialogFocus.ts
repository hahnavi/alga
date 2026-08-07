import { nextTick, onBeforeUnmount, onMounted, onUnmounted, watch, type Ref } from "vue";

const TABBABLE_SELECTOR = [
  "a[href]:not([disabled])",
  "button:not([disabled])",
  "textarea:not([disabled])",
  "input:not([disabled]):not([type='hidden'])",
  "select:not([disabled])",
  "[tabindex]:not([tabindex='-1'])",
].join(", ");

function tabbableInContainer(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll(TABBABLE_SELECTOR)).filter(
    (node): node is HTMLElement => {
      if (!(node instanceof HTMLElement)) return false;
      if (node.closest("[inert]")) return false;
      if (node.getAttribute("aria-hidden") === "true") return false;
      return node.getClientRects().length > 0;
    },
  );
}

function isBodyOrDocument(el: HTMLElement): boolean {
  return el === document.body || el === document.documentElement;
}

function runTabTrap(root: HTMLElement, e: KeyboardEvent): void {
  if (e.key !== "Tab") return;
  const tabbables = tabbableInContainer(root);
  if (tabbables.length === 0) {
    e.preventDefault();
    return;
  }

  const first = tabbables[0]!;
  const last = tabbables[tabbables.length - 1]!;
  const active = document.activeElement;

  const activeInRoot = active instanceof Node && root.contains(active);
  if (!activeInRoot) {
    e.preventDefault();
    first.focus();
    return;
  }

  if (e.shiftKey) {
    if (active === first) {
      e.preventDefault();
      last.focus();
    }
  } else if (active === last) {
    e.preventDefault();
    first.focus();
  }
}

function focusInitial(root: HTMLElement, getInitialFocus?: () => HTMLElement | null): void {
  const initial = getInitialFocus?.() ?? null;
  const pick =
    initial != null && root.contains(initial) ? initial : (tabbableInContainer(root)[0] ?? null);
  pick?.focus();
}

/**
 * When `open` is true: traps Tab within the container, focuses `getInitialFocus()` or the first
 * tabbable, and remembers the previously focused element for restore.
 * When `open` becomes false: restores focus to that element if it is still connected.
 */
export function useModalFocusTrap(
  open: Ref<boolean>,
  getContainer: () => HTMLElement | null | undefined,
  options?: { getInitialFocus?: () => HTMLElement | null },
): void {
  let previousFocus: HTMLElement | null = null;
  let trapping = false;

  function onDocumentKeydown(e: KeyboardEvent) {
    if (!open.value || !trapping) return;
    const root = getContainer() ?? null;
    if (root == null || !root.isConnected) return;
    runTabTrap(root, e);
  }

  watch(open, async (isOpen) => {
    if (isOpen) {
      const ae = document.activeElement;
      if (ae instanceof HTMLElement && !isBodyOrDocument(ae)) {
        previousFocus = ae;
      } else {
        previousFocus = null;
      }

      await nextTick();
      await new Promise<void>((r) => requestAnimationFrame(() => r()));

      let root = getContainer() ?? null;
      if (root == null) {
        await nextTick();
        root = getContainer() ?? null;
      }

      if (root) {
        trapping = true;
        document.addEventListener("keydown", onDocumentKeydown, true);
        focusInitial(root, options?.getInitialFocus);
      }
    } else {
      trapping = false;
      document.removeEventListener("keydown", onDocumentKeydown, true);
      const target = previousFocus;
      previousFocus = null;
      await nextTick();
      requestAnimationFrame(() => {
        if (target && document.contains(target) && document.body.contains(target)) {
          target.focus({ preventScroll: true });
        }
      });
    }
  });

  onUnmounted(() => {
    document.removeEventListener("keydown", onDocumentKeydown, true);
  });
}

/**
 * Focus management for dialogs rendered via `v-if` (mount = open, unmount = close),
 * such as the mobile bottom-sheet menus. On mount it records the active trigger,
 * traps Tab inside the container, and focuses the first tabbable; on unmount it
 * restores focus to that trigger if it is still in the document.
 */
export function useDialogFocusOnMount(
  getContainer: () => HTMLElement | null | undefined,
  options?: { getInitialFocus?: () => HTMLElement | null },
): void {
  let previousFocus: HTMLElement | null = null;
  let trapping = false;

  function onDocumentKeydown(e: KeyboardEvent) {
    if (!trapping) return;
    const root = getContainer() ?? null;
    if (root == null || !root.isConnected) return;
    runTabTrap(root, e);
  }

  onMounted(async () => {
    const ae = document.activeElement;
    if (ae instanceof HTMLElement && !isBodyOrDocument(ae)) {
      previousFocus = ae;
    } else {
      previousFocus = null;
    }

    await nextTick();
    await new Promise<void>((r) => requestAnimationFrame(() => r()));

    let root = getContainer() ?? null;
    if (root == null) {
      await nextTick();
      root = getContainer() ?? null;
    }

    if (root) {
      trapping = true;
      document.addEventListener("keydown", onDocumentKeydown, true);
      focusInitial(root, options?.getInitialFocus);
    }
  });

  onBeforeUnmount(() => {
    trapping = false;
    document.removeEventListener("keydown", onDocumentKeydown, true);
    const target = previousFocus;
    previousFocus = null;
    if (target && document.contains(target) && document.body.contains(target)) {
      target.focus({ preventScroll: true });
    }
  });
}
