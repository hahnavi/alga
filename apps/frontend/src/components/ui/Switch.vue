<script setup lang="ts">
const props = defineProps<{
  modelValue: boolean;
  disabled?: boolean;
  id?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

function toggle() {
  if (props.disabled) return;
  emit("update:modelValue", !props.modelValue);
}
</script>

<template>
  <button
    :id="id"
    type="button"
    role="switch"
    :aria-checked="modelValue"
    :disabled="disabled"
    class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
    :class="modelValue ? 'bg-[var(--accent)]' : 'bg-[var(--border-input)]'"
    @click="toggle"
  >
    <span
      aria-hidden="true"
      class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-[var(--bg-primary)] shadow ring-0 transition duration-200 ease-in-out"
      :class="modelValue ? 'translate-x-4' : 'translate-x-0'"
    />
  </button>
</template>
