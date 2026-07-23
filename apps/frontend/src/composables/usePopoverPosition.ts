import {
  nextTick,
  onBeforeUnmount,
  onMounted,
  reactive,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from "vue";

export type PopoverPlacement =
  | "top-left"
  | "top-right"
  | "bottom-left"
  | "bottom-right"
  | "right"
  | "left";

type PopoverPosition = {
  top?: number;
  right?: number;
  bottom?: number;
  left?: number;
};

type UsePopoverPositionOptions = {
  trigger: Ref<HTMLElement | null>;
  isOpen: Ref<boolean>;
  placement: MaybeRefOrGetter<PopoverPlacement>;
  offset?: number;
  contentRef?: Ref<HTMLElement | null>;
};

export function usePopoverPosition(options: UsePopoverPositionOptions) {
  const { trigger, isOpen } = options;
  const offset = options.offset ?? 4;
  const position = reactive<PopoverPosition>({});

  function compute() {
    const el = trigger.value;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const place = toValue(options.placement);

    switch (place) {
      case "top-left":
        position.top = undefined;
        position.right = undefined;
        position.bottom = vh - rect.top + offset;
        position.left = rect.left;
        break;
      case "top-right":
        position.top = undefined;
        position.right = vw - rect.right;
        position.bottom = vh - rect.top + offset;
        position.left = undefined;
        break;
      case "bottom-left":
        position.top = rect.bottom + offset;
        position.right = undefined;
        position.bottom = undefined;
        position.left = rect.left;
        break;
      case "bottom-right":
        position.top = rect.bottom + offset;
        position.right = vw - rect.right;
        position.bottom = undefined;
        position.left = undefined;
        break;
      case "right":
        position.top = rect.top;
        position.right = undefined;
        position.bottom = undefined;
        position.left = rect.right + offset;
        break;
      case "left":
        position.top = rect.top;
        position.right = vw - rect.left + offset;
        position.bottom = undefined;
        position.left = undefined;
        break;
    }
  }

  function clampToViewport() {
    const content = options.contentRef?.value;
    if (!content) return;
    const rect = content.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const minMargin = offset;

    let dx = 0;
    let dy = 0;

    if (rect.right > vw) {
      dx = vw - rect.right;
    } else if (rect.left < 0) {
      dx = -rect.left;
    }

    if (rect.bottom > vh) {
      dy = vh - rect.bottom;
    } else if (rect.top < 0) {
      dy = -rect.top;
    }

    if (dx === 0 && dy === 0) return;

    if (position.left !== undefined) {
      const target = position.left + dx;
      const maxLeft = Math.max(minMargin, vw - rect.width - minMargin);
      position.left = Math.max(minMargin, Math.min(target, maxLeft));
    }
    if (position.right !== undefined) {
      const target = position.right - dx;
      const maxRight = Math.max(minMargin, vw - rect.width - minMargin);
      position.right = Math.max(minMargin, Math.min(target, maxRight));
    }
    if (position.top !== undefined) {
      const target = position.top + dy;
      const maxTop = Math.max(minMargin, vh - rect.height - minMargin);
      position.top = Math.max(minMargin, Math.min(target, maxTop));
    }
    if (position.bottom !== undefined) {
      const target = position.bottom - dy;
      const maxBottom = Math.max(minMargin, vh - rect.height - minMargin);
      position.bottom = Math.max(minMargin, Math.min(target, maxBottom));
    }
  }

  function update() {
    compute();
    if (!options.contentRef) return;
    nextTick(() => {
      if (isOpen.value) clampToViewport();
    });
  }

  let rafScheduled = false;
  function scheduleUpdate() {
    if (!isOpen.value || rafScheduled) return;
    rafScheduled = true;
    requestAnimationFrame(() => {
      rafScheduled = false;
      if (isOpen.value) update();
    });
  }

  function onScrollOrResize() {
    scheduleUpdate();
  }

  watch(isOpen, (open) => {
    if (open) update();
  });

  watch(
    () => toValue(options.placement),
    () => {
      if (isOpen.value) update();
    },
  );

  onMounted(() => {
    window.addEventListener("scroll", onScrollOrResize, true);
    window.addEventListener("resize", onScrollOrResize);
  });

  onBeforeUnmount(() => {
    window.removeEventListener("scroll", onScrollOrResize, true);
    window.removeEventListener("resize", onScrollOrResize);
  });

  return position;
}
