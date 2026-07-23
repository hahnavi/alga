<script setup lang="ts">
import { computed, useAttrs } from "vue";

defineOptions({ inheritAttrs: false });

defineProps<{
  placeholder?: string;
  id?: string;
  required?: boolean;
  disabled?: boolean;
  error?: string;
  rows?: number | string;
  modelValue?: string;
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
  <textarea
    v-bind="forwardedAttrs"
    :id="id"
    :value="modelValue ?? ''"
    :placeholder="placeholder"
    :disabled="disabled"
    :required="required"
    :rows="rows ?? 4"
    :aria-invalid="error ? 'true' : undefined"
    class="field"
    :class="[{ 'border-[var(--border-error)]': error }, attrs.class]"
    @input="$emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
  />
</template>
