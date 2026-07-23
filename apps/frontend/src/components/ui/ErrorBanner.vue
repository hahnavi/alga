<script setup lang="ts">
import { RefreshCw } from "@lucide/vue";

withDefaults(
  defineProps<{
    message: string;
    showRetry?: boolean;
  }>(),
  { showRetry: false },
);

const emit = defineEmits<{ retry: [] }>();
</script>

<template>
  <div
    v-if="message"
    class="flex items-center justify-between gap-3 rounded-lg border border-[var(--border-error)] bg-[var(--bg-error)] p-3 text-sm text-[var(--text-error)]"
    role="alert"
  >
    <span>{{ message }}</span>
    <button
      v-if="showRetry"
      type="button"
      class="inline-flex shrink-0 cursor-pointer items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-opacity hover:opacity-80 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
      aria-label="Retry"
      @click="emit('retry')"
    >
      <RefreshCw class="h-3.5 w-3.5" aria-hidden="true" />
      Retry
    </button>
  </div>
</template>
