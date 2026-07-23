import { onBeforeUnmount } from "vue";

export function useSearchDebounce(loadFn: () => void, delayMs = 500) {
  let debounce: number | undefined;

  function scheduleSearchReload() {
    if (debounce) clearTimeout(debounce);
    debounce = setTimeout(() => {
      debounce = undefined;
      loadFn();
    }, delayMs);
  }

  onBeforeUnmount(() => {
    if (debounce) clearTimeout(debounce);
  });

  return { scheduleSearchReload };
}
