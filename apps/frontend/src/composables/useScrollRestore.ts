import { nextTick, onActivated, onDeactivated } from "vue";
import { getScrollContainer } from "@/lib/scrollContainer";

export function useScrollRestore(options?: { skipFirst?: boolean }) {
  const skipFirst = options?.skipFirst ?? true;
  let savedScrollTop = 0;
  let firstActivation = true;

  onActivated(() => {
    if (firstActivation) {
      firstActivation = false;
      if (skipFirst) return;
    }
    nextTick(() => {
      const el = getScrollContainer();
      if (el) el.scrollTop = savedScrollTop;
    });
  });

  onDeactivated(() => {
    const el = getScrollContainer();
    if (el) savedScrollTop = el.scrollTop;
  });
}
