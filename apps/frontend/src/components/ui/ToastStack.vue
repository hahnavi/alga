<script setup lang="ts">
import { useToast } from "@/lib/toast";

const { toasts, dismiss } = useToast();
</script>

<template>
  <div class="fixed right-4 top-4 z-50 flex w-80 flex-col gap-2" aria-live="polite">
    <div
      v-for="toast in toasts"
      :key="toast.id"
      class="rounded-lg border px-3 py-2 text-sm shadow-lg backdrop-blur transition-all duration-200"
      :class="
        toast.kind === 'success'
          ? 'border-[var(--border-toast-success)] bg-[var(--bg-toast-success)]'
          : toast.kind === 'error'
            ? 'border-[var(--border-toast-error)] bg-[var(--bg-toast-error)]'
            : 'border-[var(--border-toast)] bg-[var(--bg-toast)]'
      "
    >
      <div class="flex items-start justify-between gap-2">
        <p>{{ toast.message }}</p>
        <button
          class="cursor-pointer text-xs opacity-80 transition-opacity duration-150 hover:opacity-100"
          @click="dismiss(toast.id)"
        >
          Dismiss
        </button>
      </div>
    </div>
  </div>
</template>
