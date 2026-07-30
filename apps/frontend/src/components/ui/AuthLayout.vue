<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { Moon, Sun, ShieldCheck, Zap, RadioTower } from "@lucide/vue";
import { useTheme } from "@/lib/theme";
import LogoMark from "@/components/ui/LogoMark.vue";

defineProps<{
  maxWidth?: string;
}>();

const { isDark, toggle } = useTheme();

const STATUS_ROWS = [
  { name: "alert-intake", value: "operational", tone: "emerald" },
  { name: "agent-fleet", value: "12 online", tone: "emerald" },
  { name: "escalation-engine", value: "armed", tone: "blue" },
] as const;

const LOG_LINES = [
  "ack alert #4821 — hermes-01 investigating",
  "runbook matched: redis-memory-pressure",
  "sev2 mitigated in 14m 32s",
  "page rotated — on-call: platform-core",
  "draft postmortem generated for INC-317",
];

const logIndex = ref(0);
let logTimer: number | undefined;

onMounted(() => {
  logTimer = window.setInterval(() => {
    logIndex.value = (logIndex.value + 1) % LOG_LINES.length;
  }, 3200);
});

onUnmounted(() => {
  if (logTimer !== undefined) window.clearInterval(logTimer);
});
</script>

<template>
  <div class="flex min-h-screen bg-[var(--bg-primary)] text-[var(--text-primary)]">
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

          <div
            class="mt-10 max-w-md rounded-xl border border-white/10 bg-white/[0.04] p-4 font-mono text-xs backdrop-blur-sm"
          >
            <div class="flex items-center justify-between">
              <span class="flex items-center gap-2 font-medium tracking-wide text-emerald-400">
                <span class="relative flex h-2 w-2">
                  <span
                    class="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60 [animation-duration:2.2s]"
                  ></span>
                  <span class="relative inline-flex h-2 w-2 rounded-full bg-emerald-400"></span>
                </span>
                ALL SYSTEMS OPERATIONAL
              </span>
              <span class="flex items-center gap-1.5 text-[10px] tracking-widest text-slate-500">
                LIVE
                <span class="inline-block h-3 w-[5px] animate-pulse bg-[#3b8bff]"></span>
              </span>
            </div>
            <div class="my-3 border-t border-white/5"></div>
            <ul class="space-y-2">
              <li
                v-for="row in STATUS_ROWS"
                :key="row.name"
                class="flex items-center justify-between"
              >
                <span class="text-slate-400">{{ row.name }}</span>
                <span
                  class="flex items-center gap-1.5"
                  :class="row.tone === 'emerald' ? 'text-emerald-400' : 'text-[#6cabff]'"
                >
                  <span
                    class="h-1.5 w-1.5 rounded-full"
                    :class="row.tone === 'emerald' ? 'bg-emerald-400' : 'bg-[#3b8bff]'"
                  ></span>
                  {{ row.value }}
                </span>
              </li>
            </ul>
            <div class="mt-3 border-t border-white/5 pt-3">
              <Transition name="auth-log" mode="out-in">
                <p :key="logIndex" class="truncate text-slate-500">
                  <span class="text-[#3b8bff]">›</span> {{ LOG_LINES[logIndex] }}
                </p>
              </Transition>
            </div>
          </div>

          <ul class="mt-10 space-y-4">
            <li class="flex items-center gap-3.5">
              <span
                class="flex h-9 w-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04] text-[#6cabff]"
              >
                <ShieldCheck class="h-4 w-4" />
              </span>
              <span>
                <span class="block text-sm font-medium text-slate-200"
                  >SSO, RBAC & audit trail</span
                >
                <span class="block text-xs text-slate-500">Enterprise-grade access control</span>
              </span>
            </li>
            <li class="flex items-center gap-3.5">
              <span
                class="flex h-9 w-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04] text-[#6cabff]"
              >
                <Zap class="h-4 w-4" />
              </span>
              <span>
                <span class="block text-sm font-medium text-slate-200">AI triage in seconds</span>
                <span class="block text-xs text-slate-500"
                  >Agents investigate before you're paged</span
                >
              </span>
            </li>
            <li class="flex items-center gap-3.5">
              <span
                class="flex h-9 w-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04] text-[#6cabff]"
              >
                <RadioTower class="h-4 w-4" />
              </span>
              <span>
                <span class="block text-sm font-medium text-slate-200">24/7 agent coverage</span>
                <span class="block text-xs text-slate-500"
                  >Escalations never sleep, neither do we</span
                >
              </span>
            </li>
          </ul>
        </div>

        <p class="font-mono text-[11px] text-slate-600">
          © 2026 Alga — on-call & incident management
        </p>
      </div>
    </aside>

    <main class="relative flex min-w-0 flex-1 flex-col">
      <div
        class="pointer-events-none absolute inset-x-0 top-0 h-72 bg-[radial-gradient(60%_100%_at_50%_0%,var(--accent-soft),transparent)]"
        aria-hidden="true"
      ></div>

      <button
        class="absolute right-4 top-4 z-20 cursor-pointer rounded border border-[var(--btn-default-border)] bg-[var(--btn-default-bg)] p-2 text-[var(--btn-default-text)] transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
        :title="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
        @click="toggle"
      >
        <Sun v-if="isDark" class="h-4 w-4" />
        <Moon v-else class="h-4 w-4" />
      </button>

      <div class="auth-rise flex flex-col items-center pt-14 lg:hidden">
        <LogoMark class="h-11 w-11" />
        <span class="mt-3 flex flex-col items-center">
          <span class="text-lg font-semibold leading-tight tracking-tight">Alga</span>
          <span class="font-mono text-[10px] font-medium tracking-[0.28em] text-[var(--accent)]">
            OPS CONSOLE
          </span>
        </span>
      </div>

      <div class="flex flex-1 items-center justify-center px-4 py-12 sm:px-8">
        <div :class="['relative z-10 w-full', maxWidth ?? 'max-w-sm']">
          <slot />
        </div>
      </div>
    </main>
  </div>
</template>

<style scoped>
.auth-log-enter-active,
.auth-log-leave-active {
  transition:
    opacity 0.3s ease,
    transform 0.3s ease;
}
.auth-log-enter-from {
  opacity: 0;
  transform: translateY(5px);
}
.auth-log-leave-to {
  opacity: 0;
  transform: translateY(-5px);
}
</style>
