<script setup lang="ts">
import { Loader2 } from "@lucide/vue";

withDefaults(
  defineProps<{
    centered?: boolean;
    label?: string;
  }>(),
  {
    centered: false,
    label: "Loading...",
  },
);
</script>

<template>
  <div
    class="flex items-center gap-2 text-sm text-[var(--text-muted)]"
    :class="centered ? 'justify-center py-12' : ''"
  >
    <Loader2 class="h-5 w-5 animate-spin" />
    {{ label }}
  </div>
</template>
