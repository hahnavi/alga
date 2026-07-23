<script setup lang="ts">
import { Moon, Sun } from "@lucide/vue";
import { useTheme } from "@/lib/theme";
import Card from "@/components/ui/Card.vue";

defineProps<{
  maxWidth?: string;
}>();

const { isDark, toggle } = useTheme();
</script>

<template>
  <div
    class="relative flex min-h-screen items-center justify-center bg-[var(--bg-primary)] px-4 text-[var(--text-primary)]"
  >
    <button
      class="absolute right-4 top-4 cursor-pointer rounded border border-[var(--btn-default-border)] bg-[var(--btn-default-bg)] p-2 text-[var(--btn-default-text)] transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
      :title="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
      @click="toggle"
    >
      <Sun v-if="isDark" class="h-4 w-4" />
      <Moon v-else class="h-4 w-4" />
    </button>
    <Card :class="['w-full rounded-xl p-8 shadow-lg', maxWidth ?? 'max-w-sm']">
      <slot />
    </Card>
  </div>
</template>
