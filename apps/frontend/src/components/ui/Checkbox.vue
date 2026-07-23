<script setup lang="ts">
import { computed } from "vue";
import { Check, Minus } from "@lucide/vue";

const props = withDefaults(
  defineProps<{
    modelValue?: boolean;
    disabled?: boolean;
    id?: string;
    indeterminate?: boolean;
  }>(),
  {
    modelValue: false,
    disabled: false,
    indeterminate: false,
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  change: [value: boolean];
}>();

const checked = computed(() => props.modelValue === true);

function onChange(event: Event) {
  const value = (event.target as HTMLInputElement).checked;
  emit("update:modelValue", value);
  emit("change", value);
}
</script>

<template>
  <span class="relative inline-flex shrink-0 items-center justify-center">
    <input
      :id="id"
      type="checkbox"
      :checked="checked"
      :disabled="disabled"
      :indeterminate.prop="indeterminate"
      class="peer h-4 w-4 shrink-0 cursor-pointer appearance-none rounded border border-[var(--border-input)] bg-[var(--bg-input)] transition-colors hover:border-[var(--border-secondary)] checked:border-[var(--accent)] checked:bg-[var(--accent)] indeterminate:border-[var(--accent)] indeterminate:bg-[var(--accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
      @change="onChange"
    />
    <Check
      v-if="checked && !indeterminate"
      aria-hidden="true"
      class="pointer-events-none absolute h-3 w-3 text-white"
    />
    <Minus
      v-else-if="indeterminate"
      aria-hidden="true"
      class="pointer-events-none absolute h-0.5 w-2.5 text-white"
    />
  </span>
</template>
