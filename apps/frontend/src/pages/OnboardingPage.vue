<script setup lang="ts">
import { ref, computed } from "vue";
import { useRouter } from "vue-router";
import {
  ArrowLeft,
  ArrowRight,
  Bot,
  CheckCircle,
  Copy,
  Link,
  MessageSquare,
  Radio,
  ShieldCheck,
} from "@lucide/vue";
import { useAuthStore } from "@/stores/auth";
import { api } from "@/lib/api";
import { useClipboard } from "@/composables/useClipboard";
import { useToast } from "@/lib/toast";
import Button from "@/components/ui/Button.vue";
import AuthLayout from "@/components/ui/AuthLayout.vue";
import Tabs, { type Tab } from "@/components/ui/Tabs.vue";

defineOptions({ name: "OnboardingPage" });

const auth = useAuthStore();
const router = useRouter();
const { copyToClipboard } = useClipboard();
const { push } = useToast();

const step = ref(0);
const totalSteps = 3;
const loading = ref(false);

const connectTab = ref<"webhook" | "slack" | "agent">("webhook");

const connectTabItems: Tab<typeof connectTab.value>[] = [
  { id: "webhook", label: "Webhook", icon: Link },
  { id: "slack", label: "Slack", icon: MessageSquare },
  { id: "agent", label: "Agent", icon: Bot },
];

const webhookUrl = computed(() => {
  const base = window.location.origin;
  return `${base}/webhooks/alerts`;
});

const stepLabels = ["Welcome", "Connect", "Done"];

const features = [
  {
    icon: Radio,
    title: "Alert triage",
    desc: "Ingest alerts from Grafana and webhooks, dedupe by fingerprint, and page the right people.",
  },
  {
    icon: Bot,
    title: "Agent investigations",
    desc: "SRE agents pick up investigations automatically and report findings back to you.",
  },
  {
    icon: ShieldCheck,
    title: "Incident command",
    desc: "Drive incidents from detected to resolved with SLAs, escalations, and a full audit trail.",
  },
];

async function completeOnboarding() {
  loading.value = true;
  try {
    await api.completeOnboarding();
    auth.markOnboardingCompleted();
    router.push("/");
  } catch {
    push("Failed to complete setup", "error");
  } finally {
    loading.value = false;
  }
}

function nextStep() {
  if (step.value < totalSteps - 1) step.value++;
}

function prevStep() {
  if (step.value > 0) step.value--;
}
</script>

<template>
  <AuthLayout max-width="max-w-lg">
    <div class="auth-rise mb-8">
      <div class="flex items-baseline justify-between">
        <p
          class="font-mono text-[11px] font-medium uppercase tracking-[0.22em] text-[var(--accent)]"
        >
          Step {{ step + 1 }} of {{ totalSteps }}
        </p>
        <p class="font-mono text-[11px] uppercase tracking-[0.22em] text-[var(--text-muted)]">
          {{ stepLabels[step] }}
        </p>
      </div>
      <div
        class="mt-3 flex gap-1.5"
        role="progressbar"
        aria-label="Setup progress"
        :aria-valuenow="step + 1"
        :aria-valuemin="1"
        :aria-valuemax="totalSteps"
      >
        <div
          v-for="i in totalSteps"
          :key="i"
          class="h-[3px] flex-1 overflow-hidden rounded-full bg-[var(--border-primary)]"
        >
          <div
            class="h-full rounded-full bg-[var(--accent)] transition-[width] duration-500 ease-out"
            :class="i - 1 <= step ? 'w-full' : 'w-0'"
          />
        </div>
      </div>
    </div>

    <div class="auth-rise [animation-delay:80ms]">
      <Transition name="auth-swap" mode="out-in">
        <div :key="step">
          <div v-if="step === 0">
            <p
              class="font-mono text-[11px] font-medium uppercase tracking-[0.22em] text-[var(--accent)]"
            >
              Welcome aboard
            </p>
            <h1 class="mt-2 text-[28px] font-semibold leading-tight tracking-tight">
              Let's set up your ops console
            </h1>
            <p class="mt-1.5 text-sm text-[var(--text-secondary)]">
              Alga pairs your on-call engineers with AI agents that triage, investigate, and
              coordinate response — around the clock.
            </p>

            <div class="mt-7 space-y-2.5">
              <div
                v-for="feature in features"
                :key="feature.title"
                class="group flex items-start gap-3.5 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3.5 transition-colors hover:border-[var(--border-secondary)]"
              >
                <div
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-[var(--accent-soft)] text-[var(--accent)] transition-colors group-hover:bg-[var(--accent)] group-hover:text-white"
                >
                  <component :is="feature.icon" class="h-4 w-4" />
                </div>
                <div class="min-w-0">
                  <p class="text-sm font-medium">{{ feature.title }}</p>
                  <p class="mt-0.5 text-xs leading-relaxed text-[var(--text-muted)]">
                    {{ feature.desc }}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="step === 1">
            <p
              class="font-mono text-[11px] font-medium uppercase tracking-[0.22em] text-[var(--accent)]"
            >
              Integrations
            </p>
            <h1 class="mt-2 text-[28px] font-semibold leading-tight tracking-tight">
              Connect your tools
            </h1>
            <p class="mt-1.5 text-sm text-[var(--text-secondary)]">
              Start receiving alerts and dispatching investigations. You can also finish any of
              these later.
            </p>

            <div class="mt-6">
              <Tabs
                v-model="connectTab"
                :tabs="connectTabItems"
                aria-label="Connect tools"
                id-prefix="onboarding-connect"
              >
                <template #panel-webhook>
                  <div class="space-y-4 pt-4">
                    <div>
                      <p class="text-sm font-medium">Grafana webhook URL</p>
                      <p class="mt-1 text-xs leading-relaxed text-[var(--text-muted)]">
                        Add this URL as a webhook endpoint in your Grafana alerting rules and
                        authenticate with a webhook token.
                      </p>
                      <div
                        class="mt-3 flex h-11 items-center gap-2.5 rounded border border-[var(--border-input)] bg-[var(--bg-input)] pl-3 pr-1.5 transition-colors hover:border-[var(--border-secondary)]"
                      >
                        <Link class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
                        <code class="min-w-0 flex-1 truncate font-mono text-xs">
                          {{ webhookUrl }}
                        </code>
                        <button
                          type="button"
                          class="shrink-0 cursor-pointer rounded p-1.5 text-[var(--text-muted)] transition-colors hover:bg-[var(--hover-neutral)] hover:text-[var(--text-primary)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
                          aria-label="Copy webhook URL"
                          @click="copyToClipboard(webhookUrl, 'Webhook URL copied')"
                        >
                          <Copy class="h-4 w-4" />
                        </button>
                      </div>
                    </div>
                    <p class="text-xs leading-relaxed text-[var(--text-muted)]">
                      Create a webhook token on the
                      <strong class="font-medium text-[var(--text-secondary)]">
                        Incoming Webhooks
                      </strong>
                      page, then pass it as a
                      <code
                        class="rounded bg-[var(--bg-code)] px-1 py-0.5 font-mono text-[11px] text-[var(--text-code)]"
                        >Bearer</code
                      >
                      token or
                      <code
                        class="rounded bg-[var(--bg-code)] px-1 py-0.5 font-mono text-[11px] text-[var(--text-code)]"
                        >?token=</code
                      >
                      query parameter.
                    </p>
                  </div>
                </template>

                <template #panel-slack>
                  <div class="space-y-4 pt-4">
                    <div
                      class="flex items-start gap-3.5 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3.5"
                    >
                      <div
                        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-[var(--accent-soft)] text-[var(--accent)]"
                      >
                        <MessageSquare class="h-4 w-4" />
                      </div>
                      <div class="min-w-0">
                        <p class="text-sm font-medium">Slack bot</p>
                        <p class="mt-0.5 text-xs leading-relaxed text-[var(--text-muted)]">
                          Provide a Slack bot token and signing secret on the
                          <strong class="font-medium text-[var(--text-secondary)]">
                            Communication Channels
                          </strong>
                          page. Alga posts alerts to Slack channels and receives events via the
                          Slack Events API.
                        </p>
                      </div>
                    </div>
                    <p class="text-xs leading-relaxed text-[var(--text-muted)]">
                      See
                      <code
                        class="rounded bg-[var(--bg-code)] px-1 py-0.5 font-mono text-[11px] text-[var(--text-code)]"
                      >
                        integrations/alga-slack-app
                      </code>
                      for the Slack app manifest.
                    </p>
                  </div>
                </template>

                <template #panel-agent>
                  <div class="space-y-4 pt-4">
                    <div
                      class="flex items-start gap-3.5 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3.5"
                    >
                      <div
                        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-[var(--accent-soft)] text-[var(--accent)]"
                      >
                        <Bot class="h-4 w-4" />
                      </div>
                      <div class="min-w-0">
                        <p class="text-sm font-medium">SRE agent</p>
                        <p class="mt-0.5 text-xs leading-relaxed text-[var(--text-muted)]">
                          Create an agent token on the
                          <strong class="font-medium text-[var(--text-secondary)]">Agents</strong>
                          page to connect a Hermes or OpenClaw SRE agent. Agents connect via SSE and
                          receive investigation dispatches automatically.
                        </p>
                      </div>
                    </div>
                    <p class="text-xs leading-relaxed text-[var(--text-muted)]">
                      The agent uses the token to authenticate to the SSE stream at
                      <code
                        class="rounded bg-[var(--bg-code)] px-1 py-0.5 font-mono text-[11px] text-[var(--text-code)]"
                      >
                        /api/v1/agent/events
                      </code>
                      and the REST API at
                      <code
                        class="rounded bg-[var(--bg-code)] px-1 py-0.5 font-mono text-[11px] text-[var(--text-code)]"
                      >
                        /api/v1/agent/* </code
                      >.
                    </p>
                  </div>
                </template>
              </Tabs>
            </div>
          </div>

          <div v-else>
            <div
              class="flex h-12 w-12 items-center justify-center rounded-lg bg-[var(--bg-badge-resolved)]"
            >
              <CheckCircle class="h-6 w-6 text-[var(--text-success)]" />
            </div>
            <p
              class="mt-5 font-mono text-[11px] font-medium uppercase tracking-[0.22em] text-[var(--accent)]"
            >
              Ready
            </p>
            <h1 class="mt-2 text-[28px] font-semibold leading-tight tracking-tight">
              You're all set
            </h1>
            <p class="mt-1.5 text-sm text-[var(--text-secondary)]">
              Alga is ready to receive alerts and dispatch investigations. Head to the dashboard to
              get started — add more channels, webhooks, and agents any time.
            </p>
            <Button
              variant="primary"
              class="group mt-8 h-11 w-full text-sm font-semibold"
              :loading="loading"
              @click="completeOnboarding"
            >
              Go to dashboard
              <ArrowRight class="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
            </Button>
          </div>

          <div v-if="step < 2" class="mt-8 flex items-center justify-between gap-3">
            <Button v-if="step > 0" variant="outline" class="group h-11" @click="prevStep">
              <ArrowLeft class="h-4 w-4 transition-transform group-hover:-translate-x-0.5" />
              Back
            </Button>
            <span
              v-else
              class="font-mono text-[11px] uppercase tracking-[0.18em] text-[var(--text-muted)]"
            >
              ~1 min setup
            </span>
            <Button variant="primary" class="group h-11 text-sm font-semibold" @click="nextStep">
              {{ step === 1 ? "Finish setup" : "Continue" }}
              <ArrowRight class="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
            </Button>
          </div>
        </div>
      </Transition>
    </div>
  </AuthLayout>
</template>
