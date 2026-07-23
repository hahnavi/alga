<script setup lang="ts">
withDefaults(
  defineProps<{
    message: string;
    title?: string;
  }>(),
  { title: "" },
);
</script>

<template>
  <div class="flex flex-col items-center gap-2 py-8 text-[var(--text-muted)]">
    <slot name="icon" />
    <h3 v-if="title" class="text-base font-semibold text-[var(--text-primary)]">{{ title }}</h3>
    <p class="text-sm">{{ message }}</p>
    <div v-if="$slots.actions" class="mt-2 flex flex-wrap items-center justify-center gap-2">
      <slot name="actions" />
    </div>
    <slot name="footer" />
  </div>
</template>
