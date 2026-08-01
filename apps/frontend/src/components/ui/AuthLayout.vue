<script setup lang="ts">
import { Moon, Sun } from "@lucide/vue";
import { useTheme } from "@/lib/theme";
import LogoMark from "@/components/ui/LogoMark.vue";

defineProps<{
  maxWidth?: string;
}>();

const { isDark, toggle } = useTheme();
const year = new Date().getFullYear();
</script>

<template>
  <div class="flex min-h-dvh bg-[var(--bg-primary)] text-[var(--text-primary)]">
    <aside
      class="sticky top-0 hidden h-screen w-[44%] max-w-[620px] flex-col overflow-hidden bg-[#050a18] p-10 text-slate-300 lg:flex xl:p-14"
    >
      <div class="auth-grid pointer-events-none absolute inset-0" aria-hidden="true"></div>
      <div
        class="pointer-events-none absolute -left-32 -top-32 h-96 w-96 rounded-full bg-[#1d4ed8]/25 blur-[110px]"
        aria-hidden="true"
      ></div>
      <div
        class="pointer-events-none absolute -bottom-36 -right-28 h-[420px] w-[420px] rounded-full bg-[#0ea5e9]/15 blur-[120px]"
        aria-hidden="true"
      ></div>
      <div class="pointer-events-none absolute -bottom-24 -right-24" aria-hidden="true">
        <div class="h-80 w-80 rounded-full border border-white/5"></div>
        <div
          class="absolute -bottom-12 -right-12 h-56 w-56 rounded-full border border-white/10"
        ></div>
        <div class="auth-radar absolute inset-0 rounded-full border border-[#3b8bff]/30"></div>
      </div>

      <div class="relative z-10 flex h-full flex-col">
        <div class="flex items-center gap-3">
          <LogoMark class="h-10 w-10" />
          <span class="flex flex-col">
            <span class="text-lg font-semibold leading-tight tracking-tight text-white">Alga</span>
            <span class="font-mono text-[10px] font-medium tracking-[0.28em] text-[#6cabff]">
              OPS CONSOLE
            </span>
          </span>
        </div>

        <div class="my-auto py-12">
          <p class="font-mono text-xs font-medium tracking-[0.28em] text-[#3b8bff]">
            // INCIDENT COMMAND
          </p>
          <h2
            class="mt-4 max-w-md text-4xl font-semibold leading-[1.08] tracking-tight text-white xl:text-[44px]"
          >
            Every incident, under command.
          </h2>
          <p class="mt-4 max-w-md text-[15px] leading-relaxed text-slate-400">
            Alga pairs your on-call engineers with AI agents that triage, investigate, and
            coordinate response — around the clock.
          </p>
        </div>

        <p class="font-mono text-[11px] text-slate-600">
          © {{ year }} Alga — on-call & incident management
        </p>
      </div>
    </aside>

    <main class="relative flex min-w-0 flex-1 flex-col">
      <div
        class="pointer-events-none absolute inset-x-0 top-0 h-72 bg-[radial-gradient(60%_100%_at_50%_0%,var(--accent-soft),transparent)]"
        aria-hidden="true"
      ></div>

      <button
        type="button"
        class="absolute right-4 top-4 z-20 cursor-pointer rounded border border-[var(--btn-default-border)] bg-[var(--btn-default-bg)] p-2 text-[var(--btn-default-text)] transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
        :aria-label="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
        :title="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
        @click="toggle"
      >
        <Sun v-if="isDark" class="h-4 w-4" />
        <Moon v-else class="h-4 w-4" />
      </button>

      <div class="auth-rise flex flex-col items-center pt-10 lg:hidden">
        <LogoMark class="h-10 w-10" />
        <span class="mt-2.5 flex flex-col items-center">
          <span class="text-lg font-semibold leading-tight tracking-tight">Alga</span>
          <span class="font-mono text-[10px] font-medium tracking-[0.28em] text-[var(--accent)]">
            OPS CONSOLE
          </span>
        </span>
      </div>

      <div class="flex flex-1 justify-center px-4 py-8 sm:px-8 sm:py-12">
        <div :class="['relative z-10 my-auto w-full', maxWidth ?? 'max-w-sm']">
          <slot />
        </div>
      </div>
    </main>
  </div>
</template>
