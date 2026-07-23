<script setup lang="ts">
import { computed, useAttrs } from "vue";

defineOptions({ inheritAttrs: false });

defineProps<{
  placeholder?: string;
  type?: string;
  id?: string;
  required?: boolean;
  disabled?: boolean;
  error?: string;
  modelValue?: string | number | null;
}>();

defineEmits<{
  "update:modelValue": [value: string];
}>();

const attrs = useAttrs();
const forwardedAttrs = computed(() => {
  const { class: _class, ...rest } = attrs;
  return rest;
});
</script>

<template>
  <input
    v-bind="forwardedAttrs"
    :id="id"
    :value="modelValue ?? ''"
    :placeholder="placeholder"
    :type="type ?? 'text'"
    :disabled="disabled"
    :required="required"
    :aria-invalid="error ? 'true' : undefined"
    class="field"
    :class="[{ 'border-[var(--border-error)]': error }, attrs.class]"
    @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>
