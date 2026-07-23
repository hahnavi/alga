<script setup lang="ts">
import { computed, type Component } from "vue";
import { CARD_ICON_BTN_CLASS, HEADER_ICON_BTN_CLASS, ROW_ICON_BTN_CLASS } from "@/lib/uiClasses";

const props = withDefaults(
  defineProps<{
    icon: Component;
    label: string;
    size?: "sm" | "md" | "lg";
    disabled?: boolean;
    /** When true, renders an aria-disabled button that still captures pointer events. */
    ariaDisabledPreventClose?: boolean;
  }>(),
  {
    size: "md",
    disabled: false,
    ariaDisabledPreventClose: false,
  },
);

const emit = defineEmits<{
  click: [event: MouseEvent];
}>();

const sizeClass = computed(() => {
  switch (props.size) {
    case "sm":
      return CARD_ICON_BTN_CLASS;
    case "lg":
      return ROW_ICON_BTN_CLASS;
    default:
      return HEADER_ICON_BTN_CLASS;
  }
});

function onClick(e: MouseEvent) {
  if (props.disabled || props.ariaDisabledPreventClose) return;
  emit("click", e);
}
</script>

<template>
  <button
    type="button"
    :class="sizeClass"
    :aria-label="label"
    :title="label"
    :disabled="disabled"
    :aria-disabled="ariaDisabledPreventClose ? 'true' : undefined"
    @click="onClick"
  >
    <component :is="icon" class="h-4 w-4" aria-hidden="true" />
  </button>
</template>
