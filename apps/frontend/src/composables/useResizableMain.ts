import { onBeforeUnmount, onMounted, ref } from "vue";
import { safeGetItem, safeSetItem } from "@/lib/storage";

type ResizeDirection = "grow-on-left-drag" | "grow-on-right-drag";

type ResizableColumnOptions = {
  storageKey: string;
  defaultWidth: number;
  minWidth: number;
  maxWidth: number;
  step?: number;
  direction: ResizeDirection;
};

function clamp(width: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, Math.round(width)));
}

function loadStoredWidth(
  storageKey: string,
  defaultWidth: number,
  minWidth: number,
  maxWidth: number,
): number {
  const raw = safeGetItem(storageKey);
  if (!raw) return defaultWidth;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) ? clamp(parsed, minWidth, maxWidth) : defaultWidth;
}

function storeWidth(storageKey: string, width: number) {
  safeSetItem(storageKey, String(width));
}

function useResizableColumn(options: ResizableColumnOptions) {
  const { storageKey, defaultWidth, minWidth, maxWidth, step = 24, direction } = options;

  const width = ref(defaultWidth);
  const resizing = ref(false);
  let startX = 0;
  let startWidth = defaultWidth;
  let previousCursor = "";
  let previousUserSelect = "";

  function finishResize() {
    if (!resizing.value) return;
    resizing.value = false;
    document.removeEventListener("pointermove", onPointerMove);
    document.removeEventListener("pointerup", finishResize);
    document.removeEventListener("pointercancel", finishResize);
    document.body.style.cursor = previousCursor;
    document.body.style.userSelect = previousUserSelect;
    storeWidth(storageKey, width.value);
  }

  function onPointerMove(event: PointerEvent) {
    if (!resizing.value) return;
    const delta =
      direction === "grow-on-right-drag" ? event.clientX - startX : startX - event.clientX;
    width.value = clamp(startWidth + delta, minWidth, maxWidth);
  }

  function startResize(event: PointerEvent) {
    if (event.button !== 0 || !window.matchMedia("(min-width: 1024px)").matches) return;
    event.preventDefault();
    startX = event.clientX;
    startWidth = width.value;
    resizing.value = true;
    previousCursor = document.body.style.cursor;
    previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    document.addEventListener("pointermove", onPointerMove);
    document.addEventListener("pointerup", finishResize);
    document.addEventListener("pointercancel", finishResize);
  }

  function nudgeWidth(direction: "narrower" | "wider") {
    const next = direction === "wider" ? width.value + step : width.value - step;
    width.value = clamp(next, minWidth, maxWidth);
    storeWidth(storageKey, width.value);
  }

  onMounted(() => {
    width.value = loadStoredWidth(storageKey, defaultWidth, minWidth, maxWidth);
  });

  onBeforeUnmount(finishResize);

  return {
    nudgeWidth,
    resizing,
    width,
    startResize,
  };
}

export function useResizableMain() {
  const { nudgeWidth, resizing, width, startResize } = useResizableColumn({
    storageKey: "alga:detail-main-width",
    defaultWidth: 1200,
    minWidth: 480,
    maxWidth: 1500,
    step: 24,
    direction: "grow-on-right-drag",
  });

  return {
    mainWidth: width,
    nudgeMainWidth: nudgeWidth,
    resizingMain: resizing,
    startMainResize: startResize,
  };
}
