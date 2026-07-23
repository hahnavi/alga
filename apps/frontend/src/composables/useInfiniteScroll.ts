import { onBeforeUnmount, ref, watch, type Ref } from "vue";

type UseInfiniteScrollOptions = {
  rootMargin?: string;
  threshold?: number;
};

export function useInfiniteScroll(
  sentinel: Ref<HTMLElement | null>,
  hasMore: () => boolean,
  onLoadMore: () => void,
  options: UseInfiniteScrollOptions = {},
) {
  const observer = ref<IntersectionObserver | null>(null);
  const { rootMargin = "120px 0px", threshold = 0 } = options;

  function teardown() {
    if (observer.value) {
      observer.value.disconnect();
      observer.value = null;
    }
  }

  function setup() {
    teardown();
    if (!sentinel.value || !hasMore()) return;
    observer.value = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onLoadMore();
      },
      { rootMargin, threshold },
    );
    observer.value.observe(sentinel.value);
  }

  onBeforeUnmount(teardown);

  watch([() => sentinel.value, hasMore], () => setup());

  return { setup, teardown };
}
