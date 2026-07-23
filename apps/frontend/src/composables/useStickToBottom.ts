import { nextTick, watchEffect, type Ref } from "vue";

/**
 * Stick-to-bottom scroll helper for chat-style containers.
 *
 * `stickToBottom()` scrolls to the bottom only when the user is already near
 * the bottom, so incoming messages don't yank away a user who is reading
 * history. `scrollToBottom()` forces a scroll — use it on open, initial load,
 * or right after the user sends a message.
 *
 * The container ref may be null or conditionally rendered; the scroll listener
 * is rebound as the element appears and removed when it disappears.
 */
export function useStickToBottom(
  containerRef: Ref<HTMLElement | null>,
  threshold = 80,
): { stickToBottom: () => void; scrollToBottom: () => Promise<void> } {
  let stuck = true;

  function recompute() {
    const el = containerRef.value;
    if (!el) return;
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    stuck = distanceFromBottom <= threshold;
  }

  function onScroll() {
    recompute();
  }

  watchEffect(
    (onCleanup) => {
      const el = containerRef.value;
      if (!el) return;
      recompute();
      el.addEventListener("scroll", onScroll, { passive: true });
      onCleanup(() => el.removeEventListener("scroll", onScroll));
    },
    { flush: "post" },
  );

  function scrollToBottom(): Promise<void> {
    stuck = true;
    return nextTick().then(() => {
      const el = containerRef.value;
      if (el) el.scrollTop = el.scrollHeight;
    });
  }

  function stickToBottom(): void {
    if (!stuck) return;
    void scrollToBottom();
  }

  return { stickToBottom, scrollToBottom };
}
