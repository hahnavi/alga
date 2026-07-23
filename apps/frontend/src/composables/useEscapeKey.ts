import { onBeforeUnmount, onMounted } from "vue";

export function useEscapeKey(handler: () => void, when: () => boolean) {
  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && when()) handler();
  }

  onMounted(() => {
    document.addEventListener("keydown", onKeydown);
  });

  onBeforeUnmount(() => {
    document.removeEventListener("keydown", onKeydown);
  });
}
