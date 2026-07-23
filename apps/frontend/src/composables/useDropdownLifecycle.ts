import { onBeforeUnmount, type Ref, watch } from "vue";

export function useDropdownLifecycle(
  isOpen: Ref<boolean>,
  rootRef: Ref<HTMLElement | null>,
  contentRef?: Ref<HTMLElement | null>,
) {
  function close() {
    isOpen.value = false;
  }

  function onDocPointerDown(ev: PointerEvent) {
    const root = rootRef.value;
    if (root && root.contains(ev.target as Node)) return;
    // The floating content may be teleported outside rootRef (e.g. <Teleport to="body">).
    // Pointer events inside it must not close the menu, or inner clicks never fire.
    const content = contentRef?.value;
    if (content && content.contains(ev.target as Node)) return;
    close();
  }

  function onDocKeydown(ev: KeyboardEvent) {
    if (ev.key === "Escape") close();
  }

  watch(
    isOpen,
    (open) => {
      if (open) {
        document.addEventListener("pointerdown", onDocPointerDown, true);
        document.addEventListener("keydown", onDocKeydown, true);
      } else {
        document.removeEventListener("pointerdown", onDocPointerDown, true);
        document.removeEventListener("keydown", onDocKeydown, true);
      }
    },
    { immediate: true },
  );

  onBeforeUnmount(() => {
    document.removeEventListener("pointerdown", onDocPointerDown, true);
    document.removeEventListener("keydown", onDocKeydown, true);
  });
}
