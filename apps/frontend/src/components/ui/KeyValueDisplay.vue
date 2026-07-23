<script setup lang="ts">
defineProps<{
  data: Record<string, string>;
}>();
</script>

<template>
  <div v-for="(value, key) in data" :key="key">
    <span class="font-medium">{{ key }}:</span>
    <span class="text-[var(--text-secondary)]">{{ value }}</span>
  </div>
</template>
