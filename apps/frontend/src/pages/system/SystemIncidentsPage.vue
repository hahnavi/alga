<script setup lang="ts">
import { computed, onMounted } from "vue";
import { FileText, Hash } from "@lucide/vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Switch from "@/components/ui/Switch.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import SkeletonRows from "@/components/ui/SkeletonRows.vue";
import SystemFormFooter from "@/components/system/SystemFormFooter.vue";
import {
  intervalsEqual,
  serializeIntervals,
  SUMMARY_SEVERITIES,
  useSystemConfigForm,
  type SystemForm,
} from "@/composables/useSystemConfigForm";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "SystemIncidentsPage" });

const FIELDS: ReadonlyArray<keyof SystemForm> = [
  "incident_summary_enabled",
  "incident_summary_interval",
  "slack_incident_channels_enabled",
  "slack_incident_channel_visibility",
  "slack_incident_channel_trigger_status",
  "slack_incident_channel_archive_on_close",
];

const {
  form,
  original,
  isSlackConfigured,
  loading,
  error,
  loadConfig,
  saving,
  saveError,
  isDirty,
  changedFields,
  discard,
  save,
  loadSlackStatus,
} = useSystemConfigForm();

usePageHeaderActions({
  title: "Incidents",
  titleIcon: FileText,
  showAdd: false,
});

const intervalsDirty = computed(() => {
  const orig = original.value;
  if (!orig) return false;
  return !intervalsEqual(form.value.incident_summary_intervals, orig.incident_summary_intervals);
});

const dirty = computed(() => isDirty(FIELDS) || intervalsDirty.value);

function buildPayload(): Record<string, unknown> {
  const payload = changedFields(FIELDS);
  if (intervalsDirty.value) {
    payload.incident_summary_intervals = serializeIntervals(form.value.incident_summary_intervals);
  }
  return payload;
}

function discardAll(): void {
  discard(FIELDS);
  const orig = original.value;
  if (orig) form.value.incident_summary_intervals = { ...orig.incident_summary_intervals };
}

onMounted(() => {
  void loadConfig();
  loadSlackStatus();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner :message="error" />

    <SkeletonRows v-if="loading" :count="6" />

    <template v-else-if="original">
      <p class="text-sm text-[var(--text-muted)]">
        Summaries and Slack channel automation for active incidents.
      </p>

      <ErrorBanner :message="saveError" />

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
              <FormLabel :for="`system-summary-${sev}`" class="capitalize">{{ sev }}</FormLabel>
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
            <Select id="system-slack-visibility" v-model="form.slack_incident_channel_visibility">
              <option value="private">Private</option>
              <option value="public">Public</option>
            </Select>
          </div>
          <div>
            <FormLabel for="system-slack-trigger">Trigger status</FormLabel>
            <Select id="system-slack-trigger" v-model="form.slack_incident_channel_trigger_status">
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
        Slack incident channels are not available because Slack is not configured. Connect Slack on
        the Communication Channels page to enable automatic channel provisioning.
      </p>

      <SystemFormFooter
        :dirty="dirty"
        :saving="saving"
        @save="save(buildPayload(), 'Incidents')"
        @discard="discardAll"
      />
    </template>
  </section>
</template>
