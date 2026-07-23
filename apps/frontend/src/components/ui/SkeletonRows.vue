<script setup lang="ts">
withDefaults(
  defineProps<{
    count?: number;
    height?: string;
  }>(),
  {
    count: 4,
    height: "h-12",
  },
);
</script>

<template>
  <div v-for="n in count" :key="n" class="animate-pulse bg-[var(--skeleton-bg)]" :class="height" />
</template>
