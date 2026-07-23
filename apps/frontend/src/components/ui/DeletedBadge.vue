<script setup lang="ts">
withDefaults(defineProps<{ title?: string }>(), { title: "This resource was deleted" });
</script>

<template>
  <span
    class="rounded bg-[var(--bg-secondary)] px-1.5 py-0.5 text-[11px] font-medium text-[var(--text-muted)]"
    :title="title"
  >
    Deleted
  </span>
</template>
