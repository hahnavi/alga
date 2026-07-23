<script setup lang="ts">
import { computed } from "vue";
import { X } from "@lucide/vue";
import { CARD_ICON_BTN_CLASS, HEADER_ICON_BTN_CLASS, ROW_ICON_BTN_CLASS } from "@/lib/uiClasses";

const props = withDefaults(
  defineProps<{
    onClick: () => void;
    disabled?: boolean;
    /** When true, the button is rendered inert (data-prevent-close) and ignores pointer activation. */
    preventClose?: boolean;
    size?: "sm" | "md" | "lg";
    label?: string;
  }>(),
  {
    disabled: false,
    preventClose: false,
    size: "sm",
    label: "Close",
  },
);

const inert = computed(() => props.disabled || props.preventClose);

const sizeClass = computed(() => {
  switch (props.size) {
    case "md":
      return HEADER_ICON_BTN_CLASS;
    case "lg":
      return ROW_ICON_BTN_CLASS;
    default:
      return CARD_ICON_BTN_CLASS;
  }
});
</script>

<template>
  <button
    type="button"
    :class="sizeClass"
    :aria-label="label"
    :title="label"
    :aria-disabled="inert ? 'true' : undefined"
    :data-prevent-close="preventClose ? 'true' : undefined"
    @click="inert ? undefined : onClick()"
  >
    <X class="h-4 w-4" aria-hidden="true" />
  </button>
</template>
