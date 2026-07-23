<script setup lang="ts">
defineProps<{
  modelValue?: string;
  id?: string;
  disabled?: boolean;
}>();
defineEmits<{
  "update:modelValue": [value: string];
}>();
</script>

<template>
  <select
    :id="id"
    :value="modelValue ?? ''"
    :disabled="disabled"
    class="select-ui field cursor-pointer"
    @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
  >
    <slot />
  </select>
</template>

<style scoped>
/* Custom chevron so we can nudge it left (inset from the right edge). */
.select-ui {
  appearance: none;
  -webkit-appearance: none;
  -moz-appearance: none;
  padding-right: 2.25rem;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='%23737373' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 0.75rem center;
  background-size: 1rem 1rem;
}
</style>

<style>
:root.light .select-ui.field {
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='%2394a3b8' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E");
}
</style>
