import { ref } from "vue";

export type ToastKind = "success" | "error" | "info";

type Toast = {
  id: number;
  message: string;
  kind: ToastKind;
};

const toasts = ref<Toast[]>([]);
const timers = new Map<number, number>();
let seq = 1;

export function useToast() {
  function push(message: string, kind: ToastKind = "info", timeoutMs = 2800) {
    const id = seq++;
    toasts.value.push({ id, message, kind });
    const timer = setTimeout(() => {
      timers.delete(id);
      dismiss(id);
    }, timeoutMs);
    timers.set(id, timer);
  }

  function dismiss(id: number) {
    const timer = timers.get(id);
    if (timer !== undefined) {
      clearTimeout(timer);
      timers.delete(id);
    }
    toasts.value = toasts.value.filter((toast) => toast.id !== id);
  }

  return { toasts, push, dismiss };
}
