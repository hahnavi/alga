import { onBeforeUnmount, type Ref } from "vue";

export type LongPressPosition = { clientX: number; clientY: number };

type UseLongPressOptions = {
  threshold?: number;
  movementThreshold?: number;
  isOpen?: Ref<boolean>;
  onTrigger: (position: LongPressPosition, event: PointerEvent) => void;
};

export function useLongPress(options: UseLongPressOptions) {
  const threshold = options.threshold ?? 500;
  const movementThreshold = options.movementThreshold ?? 10;
  let timer: number | null = null;
  let activePointerId: number | null = null;
  let startX = 0;
  let startY = 0;
  let lastTriggeredAt = 0;

  function clearTimer() {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function reset() {
    clearTimer();
    activePointerId = null;
  }

  function onPointerDown(event: PointerEvent) {
    if (!event.isPrimary) return;
    if (event.pointerType === "mouse" && event.button !== 0) return;
    if (options.isOpen?.value) return;

    activePointerId = event.pointerId;
    startX = event.clientX;
    startY = event.clientY;

    clearTimer();
    timer = setTimeout(() => {
      if (activePointerId !== event.pointerId) return;
      lastTriggeredAt = Date.now();
      options.onTrigger({ clientX: startX, clientY: startY }, event);
      reset();
    }, threshold);
  }

  function onPointerMove(event: PointerEvent) {
    if (event.pointerId !== activePointerId) return;
    const dx = Math.abs(event.clientX - startX);
    const dy = Math.abs(event.clientY - startY);
    if (dx > movementThreshold || dy > movementThreshold) {
      clearTimer();
    }
  }

  function onPointerUp(event: PointerEvent) {
    if (event.pointerId !== activePointerId) return;
    clearTimer();
    activePointerId = null;
  }

  function onPointerCancel(event: PointerEvent) {
    if (event.pointerId !== activePointerId) return;
    reset();
  }

  function shouldSuppressMouseEvent(_event: MouseEvent): boolean {
    return Date.now() - lastTriggeredAt < 350;
  }

  onBeforeUnmount(reset);

  return {
    onPointerDown,
    onPointerMove,
    onPointerUp,
    onPointerCancel,
    shouldSuppressMouseEvent,
  };
}
