<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { onMounted, ref } from "vue";
import { ShieldCheck, Pencil, Trash2 } from "@lucide/vue";
import { api, type MaintenanceWindowRecord } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Switch from "@/components/ui/Switch.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useToast } from "@/lib/toast";
import { useDelete } from "@/composables/useDelete";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import { useListFilter } from "@/composables/useListFilter";
import { labelsToText, textToLabels } from "@/lib/labels";
import { formatTime, localDatetimeToRFC3339 } from "@/lib/time";

defineOptions({ name: "MaintenanceWindowsPage" });

const { push } = useToast();

const windows = ref<MaintenanceWindowRecord[]>([]);
const loading = ref(false);
const error = ref("");
const searchInput = ref("");

const formOpen = ref(false);
const editing = ref<MaintenanceWindowRecord | null>(null);
const form = ref(blankForm());

const { submitting: saving, formError, withSubmit } = useFormSubmit();

const { showDeleteConfirm, confirmDelete, doDelete } = useDelete<MaintenanceWindowRecord>(
  async (w) => {
    await api.deleteMaintenanceWindow(w.id);
    await loadWindows();
  },
  "Maintenance window",
);

const { canWrite } = useEntityPermissions("routes");

const filteredWindows = useListFilter(
  windows,
  [
    "name",
    (w) =>
      Object.entries(w.label_matchers ?? {})
        .map(([k, v]) => `${k}=${v}`)
        .join(" "),
  ],
  searchInput,
);

type WindowForm = {
  name: string;
  start_time: string;
  end_time: string;
  label_matchers_text: string;
  enabled: boolean;
};

function blankForm(): WindowForm {
  return {
    name: "",
    start_time: "",
    end_time: "",
    label_matchers_text: "",
    enabled: true,
  };
}

function formFromWindow(w: MaintenanceWindowRecord): WindowForm {
  return {
    name: w.name,
    start_time: toLocalDatetime(w.start_time),
    end_time: toLocalDatetime(w.end_time),
    label_matchers_text: labelsToText(w.label_matchers ?? {}),
    enabled: w.enabled,
  };
}

function toLocalDatetime(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function isExpired(w: MaintenanceWindowRecord): boolean {
  if (!w.end_time) return false;
  return new Date(w.end_time) < new Date();
}

function isUpcoming(w: MaintenanceWindowRecord): boolean {
  if (!w.start_time) return false;
  return new Date(w.start_time) > new Date();
}

function statusBadge(w: MaintenanceWindowRecord): { text: string; cls: string } {
  if (!w.enabled)
    return {
      text: "Disabled",
      cls: "badge-muted",
    };
  if (isExpired(w))
    return {
      text: "Expired",
      cls: "badge-muted",
    };
  if (isUpcoming(w))
    return {
      text: "Scheduled",
      cls: "badge-blue",
    };
  return {
    text: "Active",
    cls: "badge-green",
  };
}

async function loadWindows() {
  loading.value = true;
  error.value = "";
  try {
    windows.value = await api.getMaintenanceWindows();
  } catch (e: unknown) {
    error.value = getErrorMessage(e, "Failed to load maintenance windows");
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

function openEdit(w: MaintenanceWindowRecord) {
  editing.value = w;
  form.value = formFromWindow(w);
  formOpen.value = true;
  formError.value = "";
}

async function save() {
  await withSubmit(
    async () => {
      const payload = {
        name: form.value.name,
        start_time: localDatetimeToRFC3339(form.value.start_time) ?? "",
        end_time: localDatetimeToRFC3339(form.value.end_time) ?? "",
        label_matchers: textToLabels(form.value.label_matchers_text),
        enabled: form.value.enabled,
      };
      if (editing.value) {
        await api.updateMaintenanceWindow(editing.value.id, payload);
        push("Maintenance window updated", "success");
      } else {
        await api.createMaintenanceWindow(payload);
        push("Maintenance window created", "success");
      }
      formOpen.value = false;
      editing.value = null;
      await loadWindows();
    },
    editing.value ? "Maintenance window updated" : "Maintenance window created",
  );
}

async function toggleEnabled(w: MaintenanceWindowRecord) {
  try {
    await api.updateMaintenanceWindow(w.id, { enabled: !w.enabled });
    await loadWindows();
  } catch (e: unknown) {
    push(getErrorMessage(e, "Failed to toggle"), "error");
  }
}

usePageHeaderActions({
  title: "Maintenance Windows",
  titleIcon: ShieldCheck,
  searchInput,
  searchPlaceholder: "Search maintenance windows...",
  showFilters: false,
  showAdd: canWrite,
  onAdd: openCreate,
  addLabel: "New Window",
});

onMounted(() => {
  loadWindows();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <ErrorBanner v-if="error" :message="error" @dismiss="error = ''" />

    <div class="flex flex-wrap items-center gap-2">
      <span class="text-sm text-[var(--text-muted)]"
        >{{ filteredWindows.length }}
        {{ filteredWindows.length === 1 ? "window" : "windows" }}</span
      >
      <div class="flex-1" />
    </div>

    <LoadingSpinner v-if="loading && windows.length === 0" />

    <EmptyState
      v-else-if="filteredWindows.length === 0"
      :message="
        searchInput
          ? 'No maintenance windows match your search.'
          : 'No maintenance windows configured.'
      "
    >
      <template #icon>
        <ShieldCheck class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>

    <div v-else class="space-y-3">
      <Card v-for="w in filteredWindows" :key="w.id">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0 flex-1 space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-medium">{{ w.name }}</span>
              <span :class="['badge', statusBadge(w).cls]">
                {{ statusBadge(w).text }}
              </span>
            </div>
            <div
              class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]"
            >
              <span>Start: {{ formatTime(w.start_time) }}</span>
              <span>End: {{ formatTime(w.end_time) }}</span>
            </div>
            <div v-if="Object.keys(w.label_matchers ?? {}).length > 0" class="flex flex-wrap gap-1">
              <span
                v-for="(v, k) in w.label_matchers"
                :key="k"
                class="inline-block rounded bg-[var(--bg-secondary)] px-1.5 py-0.5 text-xs font-mono text-[var(--text-secondary)]"
              >
                {{ k }}={{ v }}
              </span>
            </div>
          </div>
          <div class="flex shrink-0 items-center gap-3">
            <label class="flex items-center gap-2 text-sm text-[var(--text-muted)]">
              <Switch :modelValue="w.enabled" @update:modelValue="toggleEnabled(w)" />
            </label>
            <Button v-if="canWrite" variant="outline" size="sm" @click="openEdit(w)">
              <Pencil class="mr-1 h-3.5 w-3.5" /> Edit
            </Button>
            <Button v-if="canWrite" variant="destructive" size="sm" @click="confirmDelete(w)">
              <Trash2 class="mr-1 h-3.5 w-3.5" /> Delete
            </Button>
          </div>
        </div>
      </Card>
    </div>

    <Modal
      v-model:open="formOpen"
      :title="editing ? 'Edit Maintenance Window' : 'Create Maintenance Window'"
      :loading="saving"
      @confirm="save"
      @close="editing = null"
    >
      <ErrorBanner v-if="formError" :message="formError" @dismiss="formError = ''" />
      <div class="space-y-4">
        <div>
          <FormLabel for="maintenance-window-form-name">Name</FormLabel>
          <Input
            id="maintenance-window-form-name"
            v-model="form.name"
            type="text"
            class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 text-sm"
            placeholder="e.g. Deploy freeze"
          />
        </div>
        <div>
          <FormLabel for="window-start">Start Time</FormLabel>
          <DateTimePicker
            id="window-start"
            v-model="form.start_time"
            placeholder="Pick start date & time"
          />
        </div>
        <div>
          <FormLabel for="window-end">End Time</FormLabel>
          <DateTimePicker
            id="window-end"
            v-model="form.end_time"
            placeholder="Pick end date & time"
          />
        </div>
        <div>
          <FormLabel for="maintenance-window-form-matchers">Label Matchers</FormLabel>
          <Textarea
            id="maintenance-window-form-matchers"
            v-model="form.label_matchers_text"
            rows="3"
            class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 font-mono text-sm"
            placeholder="key=value per line, e.g.&#10;namespace=production&#10;team=platform"
          />
        </div>
        <label class="flex items-center gap-2 text-sm font-medium">
          <Switch v-model="form.enabled" />
          Enabled
        </label>
      </div>
    </Modal>

    <ConfirmDialog
      v-model:open="showDeleteConfirm"
      title="Delete Maintenance Window"
      message="Are you sure you want to delete this maintenance window? This action cannot be undone."
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDelete"
    />
  </section>
</template>
