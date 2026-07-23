<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";

defineOptions({ name: "ChatDateSeparator" });

defineProps<{ label: string }>();

const el = ref<HTMLDivElement | null>(null);
const stuck = ref(false);
// Per-instance state: previously declared at module scope, which caused
// every mounted ChatDateSeparator to clobber the previous instance's
// scroll/resize handlers, ResizeObserver, and poll interval.
const instance = {
  scrollContainer: null as HTMLElement | null,
  pollHandle: null as number | null,
  resizeObserver: null as ResizeObserver | null,
  windowScrollHandler: null as (() => void) | null,
  windowResizeHandler: null as (() => void) | null,
};

function findScrollContainer(node: HTMLElement | null): HTMLElement | null {
  let parent = node?.parentElement ?? null;
  while (parent && parent !== document.documentElement) {
    const overflowY = window.getComputedStyle(parent).overflowY;
    if (overflowY === "auto" || overflowY === "scroll" || overflowY === "overlay") {
      return parent;
    }
    parent = parent.parentElement;
  }
  return null;
}

function computeStuck(): boolean {
  const node = el.value;
  if (!node) return false;
  const rect = node.getBoundingClientRect();
  let refTop = 0;
  let padTop = 0;
  if (instance.scrollContainer) {
    refTop = instance.scrollContainer.getBoundingClientRect().top;
    padTop = parseFloat(getComputedStyle(instance.scrollContainer).paddingTop) || 0;
  }
  if (rect.bottom <= refTop) return false;
  if (rect.top > refTop + padTop + 4) return false;
  const hasScrolled =
    window.scrollY > 0 ||
    document.documentElement.scrollTop > 0 ||
    document.body.scrollTop > 0 ||
    (instance.scrollContainer !== null && instance.scrollContainer.scrollTop > 0);
  return hasScrolled;
}

function update() {
  stuck.value = computeStuck();
}

function handleScroll() {
  update();
}

function attach() {
  const node = el.value;
  if (!node) return;
  instance.scrollContainer = findScrollContainer(node);
  if (instance.scrollContainer) {
    instance.scrollContainer.addEventListener("scroll", handleScroll, { passive: true });
  }
  instance.windowScrollHandler = handleScroll;
  instance.windowResizeHandler = handleScroll;
  window.addEventListener("scroll", instance.windowScrollHandler, { passive: true });
  window.addEventListener("resize", instance.windowResizeHandler, { passive: true });
  if (typeof ResizeObserver !== "undefined") {
    instance.resizeObserver = new ResizeObserver(() => update());
    instance.resizeObserver.observe(node);
    if (instance.scrollContainer) instance.resizeObserver.observe(instance.scrollContainer);
    let p = node.parentElement;
    while (p && p !== document.body) {
      instance.resizeObserver.observe(p);
      p = p.parentElement;
    }
  }
  instance.pollHandle = window.setInterval(update, 150);
  update();
}

function detach() {
  if (instance.scrollContainer) {
    instance.scrollContainer.removeEventListener("scroll", handleScroll);
  }
  if (instance.windowScrollHandler) {
    window.removeEventListener("scroll", instance.windowScrollHandler);
    instance.windowScrollHandler = null;
  }
  if (instance.windowResizeHandler) {
    window.removeEventListener("resize", instance.windowResizeHandler);
    instance.windowResizeHandler = null;
  }
  instance.resizeObserver?.disconnect();
  instance.resizeObserver = null;
  instance.scrollContainer = null;
  if (instance.pollHandle != null) {
    clearInterval(instance.pollHandle);
    instance.pollHandle = null;
  }
}

onMounted(attach);
onBeforeUnmount(detach);

if (import.meta.hot) {
  import.meta.hot.accept();
}
</script>

<template>
  <div
    ref="el"
    class="sticky top-0 z-20 flex items-center justify-center bg-transparent py-2 transition-all duration-200"
  >
    <div
      class="flex items-center gap-3 transition-all duration-200"
      :class="
        stuck
          ? 'rounded-full bg-[var(--bg-primary)]/90 px-3 py-0.5 shadow-sm ring-1 ring-[var(--border-primary)]'
          : 'w-full'
      "
    >
      <div v-if="!stuck" class="flex-1 border-t border-[var(--border-primary)]" />
      <span class="text-xs font-medium whitespace-nowrap text-[var(--text-muted)]">{{
        label
      }}</span>
      <div v-if="!stuck" class="flex-1 border-t border-[var(--border-primary)]" />
    </div>
  </div>
</template>
