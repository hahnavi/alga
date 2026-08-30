<script setup lang="ts">
import { Sun, Moon, Monitor } from "@lucide/vue";
import { useTheme, type ThemeMode } from "@/lib/theme";
import { useFont, fontOptions } from "@/lib/font";
import Card from "@/components/ui/Card.vue";

const { mode: themeMode, setMode: setThemeMode } = useTheme();
const { current: currentFont, setFont } = useFont();

const themeOptions: { value: ThemeMode; label: string; icon: typeof Sun }[] = [
  { value: "system", label: "System", icon: Monitor },
  { value: "dark", label: "Dark", icon: Moon },
  { value: "light", label: "Light", icon: Sun },
];
</script>

<template>
  <Card class="space-y-4">
    <header>
      <h3 class="text-sm font-semibold text-[var(--text-primary)]">Theme</h3>
      <p class="text-xs text-[var(--text-muted)]">Applied immediately and saved per browser.</p>
    </header>
    <div
      class="grid gap-2"
      :style="{ gridTemplateColumns: `repeat(${themeOptions.length}, minmax(0, 1fr))` }"
    >
      <button
        v-for="opt in themeOptions"
        :key="opt.value"
        type="button"
        class="flex flex-col items-center gap-1.5 rounded-lg border px-3 py-3 text-sm transition-colors"
        :class="
          themeMode === opt.value
            ? 'border-[var(--focus-ring)] bg-[var(--focus-ring)]/10 text-[var(--text-primary)]'
            : 'border-[var(--border-primary)] text-[var(--text-secondary)] hover:border-[var(--border-secondary)] hover:text-[var(--text-primary)]'
        "
        @click="setThemeMode(opt.value)"
      >
        <component :is="opt.icon" class="h-5 w-5" />
        <span>{{ opt.label }}</span>
      </button>
    </div>
  </Card>

  <Card class="space-y-4">
    <header>
      <h3 class="text-sm font-semibold text-[var(--text-primary)]">Font</h3>
      <p class="text-xs text-[var(--text-muted)]">Interface typeface, saved per browser.</p>
    </header>
    <div
      class="grid gap-2"
      :style="{ gridTemplateColumns: `repeat(${fontOptions.length}, minmax(0, 1fr))` }"
    >
      <button
        v-for="opt in fontOptions"
        :key="opt.value"
        type="button"
        class="rounded-lg border px-3 py-3 text-sm transition-colors"
        :class="
          currentFont === opt.value
            ? 'border-[var(--focus-ring)] bg-[var(--focus-ring)]/10 text-[var(--text-primary)]'
            : 'border-[var(--border-primary)] text-[var(--text-secondary)] hover:border-[var(--border-secondary)] hover:text-[var(--text-primary)]'
        "
        :style="{ fontFamily: opt.css }"
        @click="setFont(opt.value)"
      >
        <span>{{ opt.label }}</span>
      </button>
    </div>
  </Card>
</template>
