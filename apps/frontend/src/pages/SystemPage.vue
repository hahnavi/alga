<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from "vue";
import type { Component } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  Bot,
  CalendarClock,
  FileText,
  Hash,
  KeyRound,
  Merge,
  RotateCcw,
  Save,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Workflow,
} from "@lucide/vue";
import { api, type SystemConfigValues } from "@/lib/api";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Select from "@/components/ui/Select.vue";
import Switch from "@/components/ui/Switch.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import SkeletonRows from "@/components/ui/SkeletonRows.vue";
import { useAsyncData } from "@/composables/useAsyncData";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { setPageHeader, clearPageHeader } from "@/lib/pageHeader";
import { formatTimeAgo } from "@/lib/time";
import Tabs, { type Tab } from "@/components/ui/Tabs.vue";

defineOptions({ name: "SystemPage" });

const route = useRoute();
const router = useRouter();

type SystemForm = {
  correlation_window: string;
  correlation_cooldown_ttl: string;
  investigation_timeout: string;
  max_concurrent_investigations: number;
  agent_presence_ttl: string;
  agent_disconnect_grace: string;
  scheduler_leader_ttl: string;
  session_expiry_hours: number;
  log_level: string;
  slack_incident_channels_enabled: boolean;
  slack_incident_channel_visibility: string;
  slack_incident_channel_trigger_status: string;
  slack_incident_channel_archive_on_close: boolean;
  incident_summary_enabled: boolean;
  incident_summary_interval: string;
  incident_summary_intervals: SummaryIntervals;

  // Authentication — Google OAuth login.
  google_oauth_enabled: boolean;
  google_client_id: string;
  google_client_secret: string;
  google_client_secret_set: boolean;
  google_oauth_redirect_url: string;

  // Authentication — single-provider OIDC SSO.
  oidc_enabled: boolean;
  oidc_issuer_url: string;
  oidc_client_id: string;
  oidc_client_secret: string;
  oidc_client_secret_set: boolean;
  oidc_scopes: string;
};

type SummaryIntervals = Record<string, string>;

const SUMMARY_SEVERITIES = ["critical", "high", "warning", "info"] as const;

const DEFAULTS: SystemForm = {
  correlation_window: "",
  correlation_cooldown_ttl: "",
  investigation_timeout: "",
  max_concurrent_investigations: 3,
  agent_presence_ttl: "",
  agent_disconnect_grace: "",
  scheduler_leader_ttl: "",
  session_expiry_hours: 24,
  log_level: "info",
  slack_incident_channels_enabled: false,
  slack_incident_channel_visibility: "private",
  slack_incident_channel_trigger_status: "active",
  slack_incident_channel_archive_on_close: true,
  incident_summary_enabled: false,
  incident_summary_interval: "",
  incident_summary_intervals: {},

  google_oauth_enabled: true,
  google_client_id: "",
  google_client_secret: "",
  google_client_secret_set: false,
  google_oauth_redirect_url: "",

  oidc_enabled: false,
  oidc_issuer_url: "",
  oidc_client_id: "",
  oidc_client_secret: "",
  oidc_client_secret_set: false,
  oidc_scopes: "openid email profile",
};

type TabId = "general" | "correlation" | "investigations" | "incidents" | "authentication";

const TABS = [
  {
    id: "general" as const,
    label: "General",
    icon: SlidersHorizontal,
    fields: ["log_level", "session_expiry_hours"] as (keyof SystemForm)[],
  },
  {
    id: "correlation" as const,
    label: "Correlation",
    icon: Merge,
    fields: ["correlation_window", "correlation_cooldown_ttl"] as (keyof SystemForm)[],
  },
  {
    id: "investigations" as const,
    label: "Investigations",
    icon: Workflow,
    fields: [
      "investigation_timeout",
      "max_concurrent_investigations",
      "agent_presence_ttl",
      "agent_disconnect_grace",
      "scheduler_leader_ttl",
    ] as (keyof SystemForm)[],
  },
  {
    id: "incidents" as const,
    label: "Incidents",
    icon: FileText,
    fields: [
      "incident_summary_enabled",
      "incident_summary_interval",
      "slack_incident_channels_enabled",
      "slack_incident_channel_visibility",
      "slack_incident_channel_trigger_status",
      "slack_incident_channel_archive_on_close",
    ] as (keyof SystemForm)[],
  },
  {
    id: "authentication" as const,
    label: "Authentication",
    icon: ShieldCheck,
    fields: [
      "google_oauth_enabled",
      "google_client_id",
      "google_client_secret",
      "google_oauth_redirect_url",
      "oidc_enabled",
      "oidc_issuer_url",
      "oidc_client_id",
      "oidc_client_secret",
      "oidc_scopes",
    ] as (keyof SystemForm)[],
  },
] satisfies ReadonlyArray<{
  id: TabId;
  label: string;
  icon: Component;
  fields: (keyof SystemForm)[];
}>;

function intervalsFromConfig(raw: Record<string, string> | undefined): SummaryIntervals {
  const out: SummaryIntervals = {};
  for (const sev of SUMMARY_SEVERITIES) {
    const v = raw?.[sev];
    out[sev] = typeof v === "string" ? v : "";
  }
  return out;
}

function intervalsEqual(a: SummaryIntervals, b: SummaryIntervals): boolean {
  const keys = new Set<string>([...Object.keys(a), ...Object.keys(b)]);
  for (const key of keys) {
    if ((a[key] ?? "") !== (b[key] ?? "")) return false;
  }
  return true;
}

function serializeIntervals(m: SummaryIntervals): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [sev, val] of Object.entries(m)) {
    const trimmed = val.trim();
    if (trimmed !== "") out[sev] = trimmed;
  }
  return out;
}

function fromConfig(cfg: SystemConfigValues): SystemForm {
  return {
    correlation_window: cfg.correlation_window ?? "",
    correlation_cooldown_ttl: cfg.correlation_cooldown_ttl ?? "",
    investigation_timeout: cfg.investigation_timeout ?? "",
    max_concurrent_investigations: cfg.max_concurrent_investigations ?? 3,
    agent_presence_ttl: cfg.agent_presence_ttl ?? "",
    agent_disconnect_grace: cfg.agent_disconnect_grace ?? "",
    scheduler_leader_ttl: cfg.scheduler_leader_ttl ?? "",
    session_expiry_hours: cfg.session_expiry_hours ?? 24,
    log_level: cfg.log_level || "info",
    slack_incident_channels_enabled: cfg.slack_incident_channels_enabled ?? false,
    slack_incident_channel_visibility: cfg.slack_incident_channel_visibility || "private",
    slack_incident_channel_trigger_status: cfg.slack_incident_channel_trigger_status || "active",
    slack_incident_channel_archive_on_close: cfg.slack_incident_channel_archive_on_close ?? true,
    incident_summary_enabled: cfg.incident_summary_enabled ?? false,
    incident_summary_interval: cfg.incident_summary_interval ?? "",
    incident_summary_intervals: intervalsFromConfig(cfg.incident_summary_intervals),

    google_oauth_enabled: cfg.google_oauth_enabled ?? true,
    google_client_id: cfg.google_client_id ?? "",
    google_client_secret: "",
    google_client_secret_set: cfg.google_client_secret_set ?? false,
    google_oauth_redirect_url: cfg.google_oauth_redirect_url ?? "",

    oidc_enabled: cfg.oidc_enabled ?? false,
    oidc_issuer_url: cfg.oidc_issuer_url ?? "",
    oidc_client_id: cfg.oidc_client_id ?? "",
    oidc_client_secret: "",
    oidc_client_secret_set: cfg.oidc_client_secret_set ?? false,
    oidc_scopes: cfg.oidc_scopes || "openid email profile",
  };
}

const form = ref<SystemForm>({ ...DEFAULTS });
const original = ref<SystemForm | null>(null);
const environment = ref("");
const updatedAt = ref("");
const isSlackConfigured = ref(false);

const {
  loading,
  error,
  reload: loadConfig,
} = useAsyncData(async () => {
  const cfg = await api.getSystemConfig();
  form.value = fromConfig(cfg);
  original.value = {
    ...form.value,
    incident_summary_intervals: { ...form.value.incident_summary_intervals },
  };
  environment.value = cfg.environment ?? "";
  updatedAt.value = cfg.updated_at ?? "";
  return cfg;
}, "Failed to load system config");

const { submitting: saving, formError: saveError, withSubmit: withSave } = useFormSubmit();

function isTabDirty(tabId: TabId): boolean {
  const orig = original.value;
  if (!orig) return false;
  const tab = TABS.find((t) => t.id === tabId);
  if (!tab) return false;
  for (const f of tab.fields) {
    if (form.value[f] !== orig[f]) return true;
  }
  if (tabId === "incidents") {
    if (!intervalsEqual(form.value.incident_summary_intervals, orig.incident_summary_intervals)) {
      return true;
    }
  }
  return false;
}

function tabChangedFields(tabId: TabId): Record<string, unknown> {
  const orig = original.value;
  if (!orig) return {};
  const tab = TABS.find((t) => t.id === tabId);
  if (!tab) return {};
  const payload: Record<string, unknown> = {};
  for (const f of tab.fields) {
    if (form.value[f] !== orig[f]) payload[f] = form.value[f];
  }
  if (tabId === "incidents") {
    if (!intervalsEqual(form.value.incident_summary_intervals, orig.incident_summary_intervals)) {
      payload.incident_summary_intervals = serializeIntervals(
        form.value.incident_summary_intervals,
      );
    }
  }
  return payload;
}

function discardTab(tabId: TabId) {
  const orig = original.value;
  if (!orig) return;
  const tab = TABS.find((t) => t.id === tabId);
  if (!tab) return;
  const next: SystemForm = { ...form.value };
  const fields: ReadonlyArray<keyof SystemForm> = tab.fields;
  for (const f of fields) {
    Object.assign(next, { [f]: orig[f] });
  }
  if (tabId === "incidents") {
    next.incident_summary_intervals = { ...orig.incident_summary_intervals };
  }
  form.value = next;
}

async function saveTab(tabId: TabId) {
  const payload = tabChangedFields(tabId);
  if (Object.keys(payload).length === 0) return;
  const label = TABS.find((t) => t.id === tabId)?.label ?? "Section";
  await withSave(async () => {
    await api.updateSystemConfig(payload as Partial<SystemConfigValues>);
    await loadConfig();
  }, `${label} settings saved`);
}

// --- Tabs ---

function initialTab(): TabId {
  const hash = route.hash.replace("#", "");
  return TABS.some((t) => t.id === hash) ? (hash as TabId) : "general";
}

const activeTab = ref<TabId>(initialTab());

const tabItems = computed<Tab<TabId>[]>(() =>
  TABS.map((t) => ({ id: t.id, label: t.label, icon: t.icon, dirty: isTabDirty(t.id) })),
);

watch(activeTab, (tabId) => {
  if (route.hash !== `#${tabId}`) {
    void router.replace({ path: route.path, hash: `#${tabId}` });
  }
});

onMounted(() => {
  setPageHeader("System", undefined, {
    titleIcon: h(Settings, {
      class: "h-5 w-5 shrink-0 text-[var(--text-muted)]",
      "aria-hidden": "true",
    }),
  });
  void loadConfig();
  api
    .getIntegrations()
    .then((integrations) => {
      isSlackConfigured.value = integrations?.slack?.provider_enabled ?? false;
    })
    .catch(() => {
      isSlackConfigured.value = false;
    });
});

onBeforeUnmount(() => {
  clearPageHeader();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />

    <SkeletonRows v-if="loading" :count="6" />

    <template v-else-if="original">
      <!-- Intro -->
      <div class="flex flex-col gap-1">
        <p class="text-sm text-[var(--text-muted)]">
          Runtime system configuration. Changes take effect immediately and persist across restarts.
        </p>
        <p v-if="updatedAt" class="text-xs text-[var(--text-muted)]">
          Last saved {{ formatTimeAgo(updatedAt) }}
        </p>
      </div>

      <Tabs
        v-model="activeTab"
        :tabs="tabItems"
        aria-label="System settings sections"
        id-prefix="system"
      >
        <ErrorBanner :message="saveError" />

        <template #panel-general>
          <Card class="space-y-4">
            <header class="flex items-start gap-3">
              <span
                class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
              >
                <SlidersHorizontal class="h-4 w-4" />
              </span>
              <div class="min-w-0">
                <h3 class="text-sm font-semibold text-[var(--text-primary)]">General</h3>
                <p class="text-xs text-[var(--text-muted)]">Core runtime and session behavior.</p>
              </div>
            </header>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <FormLabel for="system-log-level">Log Level</FormLabel>
                <Select id="system-log-level" v-model="form.log_level">
                  <option value="debug">Debug</option>
                  <option value="info">Info</option>
                  <option value="warn">Warn</option>
                  <option value="error">Error</option>
                  <option value="fatal">Fatal</option>
                </Select>
                <p class="mt-1 text-xs text-[var(--text-muted)]">Controls backend log verbosity.</p>
              </div>
              <div>
                <FormLabel for="system-session-expiry">Session Expiry (hours)</FormLabel>
                <NumberInput
                  id="system-session-expiry"
                  v-model.number="form.session_expiry_hours"
                  min="1"
                  max="720"
                  placeholder="24"
                />
                <p class="mt-1 text-xs text-[var(--text-muted)]">
                  How long user sessions remain valid (1–720 hours).
                </p>
              </div>
              <div>
                <FormLabel for="system-environment">Environment</FormLabel>
                <Input
                  id="system-environment"
                  :model-value="environment || '(not set)'"
                  disabled
                  class="opacity-60"
                />
                <p class="mt-1 text-xs text-[var(--text-muted)]">
                  Set via server config; read-only.
                </p>
              </div>
              <div v-if="updatedAt">
                <FormLabel>Last saved</FormLabel>
                <Input :model-value="formatTimeAgo(updatedAt)" disabled class="opacity-60" />
                <p class="mt-1 text-xs text-[var(--text-muted)]">
                  When the configuration was last persisted.
                </p>
              </div>
            </div>
          </Card>
        </template>

        <!-- Correlation -->
        <template #panel-correlation>
          <Card class="space-y-4">
            <header class="flex items-start gap-3">
              <span
                class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
              >
                <Merge class="h-4 w-4" />
              </span>
              <div class="min-w-0">
                <h3 class="text-sm font-semibold text-[var(--text-primary)]">Alert Correlation</h3>
                <p class="text-xs text-[var(--text-muted)]">
                  Group related alerts into shared investigations.
                </p>
              </div>
            </header>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <FormLabel for="system-correlation-window">Correlation Window</FormLabel>
                <Input
                  id="system-correlation-window"
                  v-model="form.correlation_window"
                  placeholder="5m"
                />
                <p class="mt-1 text-xs text-[var(--text-muted)]">
                  Go duration (e.g. 5m, 10m). Set 0 to disable correlation.
                </p>
              </div>
              <div>
                <FormLabel for="system-cooldown-ttl">Correlation Cooldown TTL</FormLabel>
                <Input
                  id="system-cooldown-ttl"
                  v-model="form.correlation_cooldown_ttl"
                  placeholder="30m"
                />
                <p class="mt-1 text-xs text-[var(--text-muted)]">
                  Wait before the same fingerprint can start a new investigation.
                </p>
              </div>
            </div>
          </Card>
        </template>

        <!-- Investigations -->
        <template #panel-investigations>
          <div class="space-y-4">
            <Card class="space-y-4">
              <header class="flex items-start gap-3">
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
                >
                  <Workflow class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">
                    Investigation Pipeline
                  </h3>
                  <p class="text-xs text-[var(--text-muted)]">
                    Throughput and timeouts for investigations.
                  </p>
                </div>
              </header>
              <div class="grid gap-4 sm:grid-cols-2">
                <div>
                  <FormLabel for="system-investigation-timeout">Investigation Timeout</FormLabel>
                  <Input
                    id="system-investigation-timeout"
                    v-model="form.investigation_timeout"
                    placeholder="10m"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    Max Go duration before an investigation is force-completed.
                  </p>
                </div>
                <div>
                  <FormLabel for="system-max-investigations"
                    >Max Concurrent Investigations</FormLabel
                  >
                  <NumberInput
                    id="system-max-investigations"
                    v-model.number="form.max_concurrent_investigations"
                    min="1"
                    placeholder="3"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    Cap of investigations the scheduler runs in parallel.
                  </p>
                </div>
              </div>
            </Card>

            <Card class="space-y-4">
              <header class="flex items-start gap-3">
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
                >
                  <Bot class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">Agent Settings</h3>
                  <p class="text-xs text-[var(--text-muted)]">
                    Presence and liveness detection for agents.
                  </p>
                </div>
              </header>
              <div class="grid gap-4 sm:grid-cols-2">
                <div>
                  <FormLabel for="system-agent-presence-ttl">Agent Presence TTL</FormLabel>
                  <Input
                    id="system-agent-presence-ttl"
                    v-model="form.agent_presence_ttl"
                    placeholder="90s"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    How long presence heartbeats count.
                  </p>
                </div>
                <div>
                  <FormLabel for="system-agent-disconnect-grace">Agent Disconnect Grace</FormLabel>
                  <Input
                    id="system-agent-disconnect-grace"
                    v-model="form.agent_disconnect_grace"
                    placeholder="45s"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    Buffer before a missed heartbeat marks an agent offline.
                  </p>
                </div>
              </div>
            </Card>

            <Card class="space-y-4">
              <header class="flex items-start gap-3">
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
                >
                  <CalendarClock class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">Scheduler</h3>
                  <p class="text-xs text-[var(--text-muted)]">
                    Leader election for the investigation scheduler.
                  </p>
                </div>
              </header>
              <div class="grid gap-4 sm:grid-cols-2">
                <div>
                  <FormLabel for="system-leader-ttl">Leader Lease TTL</FormLabel>
                  <Input
                    id="system-leader-ttl"
                    v-model="form.scheduler_leader_ttl"
                    placeholder="15s"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    Go duration. Set 0 for single-replica deployments.
                  </p>
                </div>
              </div>
            </Card>
          </div>
        </template>

        <!-- Incidents -->
        <template #panel-incidents>
          <div class="space-y-4">
            <Card class="space-y-4">
              <header class="flex items-start gap-3">
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
                >
                  <FileText class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">Incident Summary</h3>
                  <p class="text-xs text-[var(--text-muted)]">
                    Post recurring status summaries to active incident channels.
                  </p>
                </div>
              </header>

              <label
                class="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
              >
                <span class="min-w-0">
                  <span class="block text-sm font-medium text-[var(--text-primary)]">
                    Enable incident summaries
                  </span>
                  <span class="block text-xs text-[var(--text-muted)]">
                    Generate and post summaries while incidents are active.
                  </span>
                </span>
                <Switch v-model="form.incident_summary_enabled" />
              </label>

              <div class="grid gap-4 sm:grid-cols-2">
                <div>
                  <FormLabel for="system-summary-interval">Default Interval</FormLabel>
                  <Input
                    id="system-summary-interval"
                    v-model="form.incident_summary_interval"
                    placeholder="15m"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    Go duration (e.g. 15m, 30m). Applied when no per-severity override is set.
                  </p>
                </div>
              </div>

              <div>
                <span class="field-label">Per-severity overrides</span>
                <div class="grid gap-4 sm:grid-cols-2">
                  <div v-for="sev in SUMMARY_SEVERITIES" :key="sev">
                    <FormLabel :for="`system-summary-${sev}`" class="capitalize">{{
                      sev
                    }}</FormLabel>
                    <Input
                      :id="`system-summary-${sev}`"
                      v-model="form.incident_summary_intervals[sev]"
                      placeholder="use default"
                    />
                  </div>
                </div>
                <p class="mt-1 text-xs text-[var(--text-muted)]">
                  Override the default cadence for a severity; leave blank to use the default.
                </p>
              </div>
            </Card>

            <Card v-if="isSlackConfigured" class="space-y-4">
              <header class="flex items-start gap-3">
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
                >
                  <Hash class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">
                    Slack Incident Channels
                  </h3>
                  <p class="text-xs text-[var(--text-muted)]">
                    Automatically provision a dedicated Slack channel per incident.
                  </p>
                </div>
              </header>

              <div class="space-y-2">
                <label
                  class="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
                >
                  <span class="min-w-0">
                    <span class="block text-sm font-medium text-[var(--text-primary)]">
                      Auto-create channels
                    </span>
                    <span class="block text-xs text-[var(--text-muted)]">
                      Provision a channel when an incident opens.
                    </span>
                  </span>
                  <Switch v-model="form.slack_incident_channels_enabled" />
                </label>
                <label
                  class="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
                >
                  <span class="min-w-0">
                    <span class="block text-sm font-medium text-[var(--text-primary)]">
                      Archive on close
                    </span>
                    <span class="block text-xs text-[var(--text-muted)]">
                      Archive the channel automatically when the incident closes.
                    </span>
                  </span>
                  <Switch v-model="form.slack_incident_channel_archive_on_close" />
                </label>
              </div>

              <div class="grid gap-4 sm:grid-cols-2">
                <div>
                  <FormLabel for="system-slack-visibility">Channel visibility</FormLabel>
                  <Select
                    id="system-slack-visibility"
                    v-model="form.slack_incident_channel_visibility"
                  >
                    <option value="private">Private</option>
                    <option value="public">Public</option>
                  </Select>
                </div>
                <div>
                  <FormLabel for="system-slack-trigger">Trigger status</FormLabel>
                  <Select
                    id="system-slack-trigger"
                    v-model="form.slack_incident_channel_trigger_status"
                  >
                    <option value="active">Active</option>
                    <option value="detected">Detected</option>
                  </Select>
                </div>
              </div>
            </Card>

            <p
              v-else
              class="rounded-md border border-dashed border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-3 text-xs text-[var(--text-muted)]"
            >
              Slack incident channels are not available because Slack is not configured. Connect
              Slack on the Integrations page to enable automatic channel provisioning.
            </p>
          </div>
        </template>

        <!-- Authentication -->
        <template #panel-authentication>
          <div class="space-y-4">
            <Card class="space-y-4">
              <header class="flex items-start gap-3">
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
                >
                  <ShieldCheck class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">Google OAuth</h3>
                  <p class="text-xs text-[var(--text-muted)]">
                    Allow users to sign in with their Google account.
                  </p>
                </div>
              </header>

              <label
                class="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
              >
                <span class="min-w-0">
                  <span class="block text-sm font-medium text-[var(--text-primary)]">
                    Enable Google Sign-In
                  </span>
                  <span class="block text-xs text-[var(--text-muted)]">
                    Show the Google login option on the sign-in page.
                  </span>
                </span>
                <Switch v-model="form.google_oauth_enabled" />
              </label>

              <div class="grid gap-4 sm:grid-cols-2">
                <div>
                  <FormLabel for="system-google-client-id">Client ID</FormLabel>
                  <Input
                    id="system-google-client-id"
                    v-model="form.google_client_id"
                    placeholder="xxxxxxxx.apps.googleusercontent.com"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    Google OAuth 2.0 client ID from the Google Cloud console.
                  </p>
                </div>
                <div>
                  <FormLabel for="system-google-redirect">Redirect URL</FormLabel>
                  <Input
                    id="system-google-redirect"
                    v-model="form.google_oauth_redirect_url"
                    placeholder="https://alga.example.com/api/v1/auth/google/callback"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    Must match the authorized redirect URI in Google Cloud.
                  </p>
                </div>
                <div>
                  <FormLabel for="system-google-secret">Client Secret</FormLabel>
                  <Input
                    id="system-google-secret"
                    v-model="form.google_client_secret"
                    type="password"
                    :placeholder="
                      form.google_client_secret_set
                        ? '•••••••• (configured)'
                        : 'Enter client secret'
                    "
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    <span v-if="form.google_client_secret_set"
                      >A client secret is configured. Leave blank to keep it.</span
                    >
                    <span v-else>Stored encrypted; never displayed after saving.</span>
                  </p>
                </div>
              </div>
            </Card>

            <Card class="space-y-4">
              <header class="flex items-start gap-3">
                <span
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
                >
                  <KeyRound class="h-4 w-4" />
                </span>
                <div class="min-w-0">
                  <h3 class="text-sm font-semibold text-[var(--text-primary)]">OIDC SSO</h3>
                  <p class="text-xs text-[var(--text-muted)]">
                    Single sign-on via a standards-compliant OpenID Connect provider.
                  </p>
                </div>
              </header>

              <label
                class="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
              >
                <span class="min-w-0">
                  <span class="block text-sm font-medium text-[var(--text-primary)]">
                    Enable OIDC Sign-In
                  </span>
                  <span class="block text-xs text-[var(--text-muted)]">
                    Show the OIDC login option on the sign-in page.
                  </span>
                </span>
                <Switch v-model="form.oidc_enabled" />
              </label>

              <div class="grid gap-4 sm:grid-cols-2">
                <div class="sm:col-span-2">
                  <FormLabel for="system-oidc-issuer">Issuer URL</FormLabel>
                  <Input
                    id="system-oidc-issuer"
                    v-model="form.oidc_issuer_url"
                    placeholder="https://accounts.example.com"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    The OIDC issuer base URL (where <code>.well-known/openid-configuration</code> is
                    served).
                  </p>
                </div>
                <div>
                  <FormLabel for="system-oidc-client-id">Client ID</FormLabel>
                  <Input
                    id="system-oidc-client-id"
                    v-model="form.oidc_client_id"
                    placeholder="oidc-client-id"
                  />
                </div>
                <div>
                  <FormLabel for="system-oidc-scopes">Scopes</FormLabel>
                  <Input
                    id="system-oidc-scopes"
                    v-model="form.oidc_scopes"
                    placeholder="openid email profile"
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    Space-separated OAuth scopes to request.
                  </p>
                </div>
                <div>
                  <FormLabel for="system-oidc-secret">Client Secret</FormLabel>
                  <Input
                    id="system-oidc-secret"
                    v-model="form.oidc_client_secret"
                    type="password"
                    :placeholder="
                      form.oidc_client_secret_set ? '•••••••• (configured)' : 'Enter client secret'
                    "
                  />
                  <p class="mt-1 text-xs text-[var(--text-muted)]">
                    <span v-if="form.oidc_client_secret_set"
                      >A client secret is configured. Leave blank to keep it.</span
                    >
                    <span v-else>Stored encrypted; never displayed after saving.</span>
                  </p>
                </div>
              </div>
            </Card>
          </div>
        </template>
      </Tabs>

      <!-- Per-tab action footer: Discard (left) + Save (right) -->
      <div class="flex items-center justify-end gap-2 pt-2">
        <Button
          v-if="isTabDirty(activeTab)"
          variant="outline"
          :disabled="saving"
          @click="discardTab(activeTab)"
        >
          <RotateCcw class="h-4 w-4" />
          Discard
        </Button>
        <Button :disabled="!isTabDirty(activeTab)" :loading="saving" @click="saveTab(activeTab)">
          <Save class="h-4 w-4" />
          Save
        </Button>
      </div>
    </template>
  </section>
</template>
