<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { onMounted, ref } from "vue";
import { HeartPulse, Pencil, Trash2, RefreshCw } from "@lucide/vue";
import { api, type HeartbeatRecord } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import NumberInput from "@/components/ui/NumberInput.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Select from "@/components/ui/Select.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Switch from "@/components/ui/Switch.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import SecretDisplay from "@/components/ui/SecretDisplay.vue";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useClipboard } from "@/composables/useClipboard";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListFilter } from "@/composables/useListFilter";
import { formatTime, formatTimeAgo } from "@/lib/time";
import { labelsToText, textToLabels } from "@/lib/labels";

defineOptions({ name: "HeartbeatsPage" });

const { push } = useToast();
const { copyToClipboard } = useClipboard();

const heartbeats = ref<HeartbeatRecord[]>([]);
const loading = ref(false);
const error = ref("");
const searchInput = ref("");

const formOpen = ref(false);
const editing = ref<HeartbeatRecord | null>(null);
const form = ref(blankForm());

const tokenModalOpen = ref(false);
const tokenRevealUrl = ref("");

const { submitting: saving, formError, withSubmit } = useFormSubmit();

const { canWrite, canDelete } = useEntityPermissions("heartbeats");

const { showDeleteConfirm, confirmDelete, doDelete } = useDelete<HeartbeatRecord>(async (hb) => {
  await api.deleteHeartbeat(hb.id);
  await loadHeartbeats();
}, "Heartbeat");

const SEVERITY_OPTIONS = ["critical", "high", "warning", "info"] as const;

const filteredHeartbeats = useListFilter(
  heartbeats,
  [
    "name",
    (hb) =>
      Object.entries(hb.labels ?? {})
        .map(([k, v]) => `${k}=${v}`)
        .join(" "),
    "status",
  ],
  searchInput,
);

type HeartbeatForm = {
  name: string;
  description: string;
  interval_seconds: number;
  grace_seconds: number;
  severity: string;
  labels_text: string;
  enabled: boolean;
};

function blankForm(): HeartbeatForm {
  return {
    name: "",
    description: "",
    interval_seconds: 60,
    grace_seconds: 60,
    severity: "warning",
    labels_text: "",
    enabled: true,
  };
}

function formFromHeartbeat(hb: HeartbeatRecord): HeartbeatForm {
  return {
    name: hb.name,
    description: hb.description ?? "",
    interval_seconds: hb.interval_seconds,
    grace_seconds: hb.grace_seconds,
    severity: hb.severity || "warning",
    labels_text: labelsToText(hb.labels ?? {}),
    enabled: hb.enabled,
  };
}

function formatDuration(seconds: number): string {
  if (seconds <= 0) return "0s";
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  const h = Math.floor(seconds / 3600);
  const m = Math.round((seconds % 3600) / 60);
  return m > 0 ? `${h}h ${m}m` : `${h}h`;
}

function isOverdue(hb: HeartbeatRecord): boolean {
  if (!hb.enabled || !hb.expires_at) return false;
  return new Date(hb.expires_at) < new Date();
}

function statusBadge(hb: HeartbeatRecord): { text: string; cls: string } {
  if (!hb.enabled) return { text: "Disabled", cls: "badge-muted" };
  if (hb.status === "expired" || isOverdue(hb)) return { text: "Expired", cls: "badge-red" };
  return { text: "Healthy", cls: "badge-green" };
}

function severityBadge(severity: string): string {
  switch (severity) {
    case "critical":
      return "badge-red";
    case "high":
      return "badge-orange";
    case "warning":
      return "badge-yellow";
    default:
      return "badge-blue";
  }
}

async function loadHeartbeats() {
  loading.value = true;
  error.value = "";
  try {
    heartbeats.value = await api.getHeartbeats();
  } catch (e: unknown) {
    error.value = getErrorMessage(e, "Failed to load heartbeats");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  form.value = blankForm();
  formOpen.value = true;
  formError.value = "";
}

function openEdit(hb: HeartbeatRecord) {
  editing.value = hb;
  form.value = formFromHeartbeat(hb);
  formOpen.value = true;
  formError.value = "";
}

async function save() {
  await withSubmit(
    async () => {
      const payload = {
        name: form.value.name.trim(),
        description: form.value.description.trim(),
        interval_seconds: Number(form.value.interval_seconds),
        grace_seconds: Number(form.value.grace_seconds),
        severity: form.value.severity,
        labels: textToLabels(form.value.labels_text),
        enabled: form.value.enabled,
      };
      if (editing.value) {
        await api.updateHeartbeat(editing.value.id, payload);
        push("Heartbeat updated", "success");
        formOpen.value = false;
      } else {
        const created = await api.createHeartbeat(payload);
        push("Heartbeat created", "success");
        formOpen.value = false;
        if (created.ping_token) {
          tokenRevealUrl.value = api.heartbeatPingUrl(created.ping_token);
          tokenModalOpen.value = true;
        }
      }
      editing.value = null;
      await loadHeartbeats();
    },
    editing.value ? "Heartbeat updated" : "Heartbeat created",
  );
}

async function regenerateToken(hb: HeartbeatRecord) {
  try {
    const updated = await api.regenerateHeartbeatToken(hb.id);
    if (updated.ping_token) {
      tokenRevealUrl.value = api.heartbeatPingUrl(updated.ping_token);
      tokenModalOpen.value = true;
    }
    push("Heartbeat token regenerated", "success");
  } catch (e: unknown) {
    push(getErrorMessage(e, "Failed to regenerate token"), "error");
  }
}

async function toggleEnabled(hb: HeartbeatRecord) {
  try {
    await api.updateHeartbeat(hb.id, { enabled: !hb.enabled });
    await loadHeartbeats();
  } catch (e: unknown) {
    push(getErrorMessage(e, "Failed to toggle"), "error");
  }
}

async function copyUrl() {
  if (!tokenRevealUrl.value) return;
  await copyToClipboard(tokenRevealUrl.value);
  push("Ping URL copied", "success");
}

usePageHeaderActions({
  title: "Heartbeats",
  titleIcon: HeartPulse,
  searchInput,
  searchPlaceholder: "Search heartbeats...",
  showFilters: false,
  showAdd: canWrite,
  onAdd: openCreate,
  addLabel: "New Heartbeat",
});

onMounted(() => {
  loadHeartbeats();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner v-if="error" :message="error" @dismiss="error = ''" />

    <p class="text-sm text-[var(--text-muted)]">
      Heartbeats alert you when a system stops checking in. Configure a ping URL and have your cron
      jobs, workers, or daemons hit it on schedule — if a ping is missed past its grace period, Alga
      fires an alert.
    </p>

    <div class="flex flex-wrap items-center gap-2">
      <span class="text-sm text-[var(--text-muted)]"
        >{{ filteredHeartbeats.length }}
        {{ filteredHeartbeats.length === 1 ? "heartbeat" : "heartbeats" }}</span
      >
      <div class="flex-1" />
    </div>

    <LoadingSpinner v-if="loading && heartbeats.length === 0" />

    <EmptyState
      v-else-if="filteredHeartbeats.length === 0"
      :message="searchInput ? 'No heartbeats match your search.' : 'No heartbeats configured.'"
    >
      <template #icon>
        <HeartPulse class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>

    <div v-else class="space-y-3">
      <Card v-for="hb in filteredHeartbeats" :key="hb.id">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0 flex-1 space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-medium">{{ hb.name }}</span>
              <span :class="['badge', statusBadge(hb).cls]">{{ statusBadge(hb).text }}</span>
              <span :class="['badge', severityBadge(hb.severity)]">{{ hb.severity }}</span>
            </div>
            <p v-if="hb.description" class="text-sm text-[var(--text-secondary)]">
              {{ hb.description }}
            </p>
            <div
              class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]"
            >
              <span>Interval: {{ formatDuration(hb.interval_seconds) }}</span>
              <span>Grace: {{ formatDuration(hb.grace_seconds) }}</span>
              <span v-if="hb.last_ping_at"> Last ping: {{ formatTimeAgo(hb.last_ping_at) }} </span>
              <span v-else>Last ping: never</span>
              <span v-if="hb.expires_at"> Next deadline: {{ formatTime(hb.expires_at) }} </span>
            </div>
            <div v-if="Object.keys(hb.labels ?? {}).length > 0" class="flex flex-wrap gap-1">
              <span
                v-for="(v, k) in hb.labels"
                :key="k"
                class="inline-block rounded bg-[var(--bg-secondary)] px-1.5 py-0.5 text-xs font-mono text-[var(--text-secondary)]"
              >
                {{ k }}={{ v }}
              </span>
            </div>
          </div>
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <label class="flex items-center gap-2 text-sm text-[var(--text-muted)]">
              <Switch :modelValue="hb.enabled" @update:modelValue="toggleEnabled(hb)" />
            </label>
            <Button
              v-if="canWrite"
              variant="outline"
              size="sm"
              title="Generate a new ping token"
              @click="regenerateToken(hb)"
            >
              <RefreshCw class="mr-1 h-3.5 w-3.5" /> Token
            </Button>
            <Button v-if="canWrite" variant="outline" size="sm" @click="openEdit(hb)">
              <Pencil class="mr-1 h-3.5 w-3.5" /> Edit
            </Button>
            <Button v-if="canDelete" variant="destructive" size="sm" @click="confirmDelete(hb)">
              <Trash2 class="mr-1 h-3.5 w-3.5" /> Delete
            </Button>
          </div>
        </div>
      </Card>
    </div>

    <Modal
      v-model:open="formOpen"
      :title="editing ? 'Edit Heartbeat' : 'Create Heartbeat'"
      :loading="saving"
      @confirm="save"
      @close="editing = null"
    >
      <ErrorBanner v-if="formError" :message="formError" @dismiss="formError = ''" />
      <div class="space-y-4">
        <div>
          <FormLabel for="heartbeat-form-name">Name</FormLabel>
          <Input
            id="heartbeat-form-name"
            v-model="form.name"
            type="text"
            class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 text-sm"
            placeholder="e.g. nightly-backup-job"
          />
        </div>
        <div>
          <FormLabel for="heartbeat-form-description">Description</FormLabel>
          <Input
            id="heartbeat-form-description"
            v-model="form.description"
            type="text"
            class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 text-sm"
            placeholder="What this heartbeat monitors"
          />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <FormLabel for="heartbeat-form-interval">Interval (seconds)</FormLabel>
            <NumberInput
              id="heartbeat-form-interval"
              v-model.number="form.interval_seconds"
              min="1"
              class="w-full"
            />
          </div>
          <div>
            <FormLabel for="heartbeat-form-grace">Grace (seconds)</FormLabel>
            <NumberInput
              id="heartbeat-form-grace"
              v-model.number="form.grace_seconds"
              min="0"
              class="w-full"
            />
          </div>
        </div>
        <div>
          <FormLabel for="heartbeat-form-severity">Severity</FormLabel>
          <Select
            id="heartbeat-form-severity"
            v-model="form.severity"
            class="w-full rounded-md p-2 text-sm"
          >
            <option v-for="s in SEVERITY_OPTIONS" :key="s" :value="s">{{ s }}</option>
          </Select>
        </div>
        <div>
          <FormLabel for="heartbeat-form-labels">Labels (attached to generated alerts)</FormLabel>
          <Textarea
            id="heartbeat-form-labels"
            v-model="form.labels_text"
            rows="3"
            class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 font-mono text-sm"
            placeholder="key=value per line, e.g.&#10;service=payments&#10;team=platform"
          />
        </div>
        <label class="flex items-center gap-2 text-sm font-medium">
          <Switch v-model="form.enabled" />
          Enabled
        </label>
      </div>
    </Modal>

    <Modal v-model:open="tokenModalOpen" title="Heartbeat Ping URL" @close="tokenRevealUrl = ''">
      <div class="space-y-3">
        <p class="text-sm text-[var(--text-secondary)]">
          Have your system send a request to this URL on schedule. A missed ping past the grace
          period fires an alert.
        </p>
        <p class="text-xs text-[var(--text-muted)]">
          This URL contains the ping token and is shown only once. Store it now; you can regenerate
          it later if compromised.
        </p>
        <SecretDisplay :secret="tokenRevealUrl" @copy="copyUrl" />
        <pre
          class="overflow-x-auto rounded-md bg-[var(--bg-code)] p-2 text-xs text-[var(--text-code)]"
        ><code>curl {{ tokenRevealUrl }}</code></pre>
      </div>
    </Modal>

    <ConfirmDialog
      v-model:open="showDeleteConfirm"
      title="Delete Heartbeat"
      message="Are you sure you want to delete this heartbeat? This action cannot be undone."
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDelete"
    />
  </section>
</template>
