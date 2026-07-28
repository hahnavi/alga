<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ArrowLeft, Plus, Trash2, RefreshCw } from "@lucide/vue";
import {
  api,
  type StatusPageView,
  type StatusPageComponentRecord,
  type ComponentStatus,
} from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useToast } from "@/lib/toast";
import { formatTime, formatTimeAgo } from "@/lib/time";

defineOptions({ name: "StatusPageViewPage" });

const route = useRoute();
const router = useRouter();
const { push } = useToast();
const { canWrite, canDelete } = useEntityPermissions("statuspages");

const view = ref<StatusPageView | null>(null);
const loading = ref(false);
const error = ref("");

const newComponentName = ref("");
const newComponentStatus = ref<ComponentStatus>("operational");
const adding = ref(false);
const removeTarget = ref<StatusPageComponentRecord | null>(null);
const removing = ref(false);

const COMPONENT_STATUS_OPTIONS: ComponentStatus[] = [
  "operational",
  "degraded",
  "partial_outage",
  "major_outage",
  "maintenance",
];

const slug = computed(() => String(route.params.slug ?? ""));

function statusMeta(status: string): { label: string; dot: string; banner: string; badge: string } {
  switch (status) {
    case "operational":
      return {
        label: "All Systems Operational",
        dot: "bg-green-500",
        banner: "border-green-500/40 bg-green-500/10",
        badge: "badge-green",
      };
    case "degraded":
      return {
        label: "Degraded Performance",
        dot: "bg-yellow-500",
        banner: "border-yellow-500/40 bg-yellow-500/10",
        badge: "badge-yellow",
      };
    case "partial_outage":
      return {
        label: "Partial Outage",
        dot: "bg-orange-500",
        banner: "border-orange-500/40 bg-orange-500/10",
        badge: "badge-orange",
      };
    case "major_outage":
      return {
        label: "Major Outage",
        dot: "bg-red-500",
        banner: "border-red-500/40 bg-red-500/10",
        badge: "badge-red",
      };
    case "maintenance":
      return {
        label: "Under Maintenance",
        dot: "bg-blue-500",
        banner: "border-blue-500/40 bg-blue-500/10",
        badge: "badge-blue",
      };
    default:
      return {
        label: status,
        dot: "bg-gray-500",
        banner: "border-[var(--border-primary)] bg-[var(--bg-secondary)]",
        badge: "badge-muted",
      };
  }
}

function incidentStatusLabel(status: string): { text: string; cls: string } {
  switch (status) {
    case "active":
      return { text: "Investigating", cls: "badge-red" };
    case "mitigated":
      return { text: "Mitigated", cls: "badge-yellow" };
    case "triaging":
      return { text: "Triaging", cls: "badge-orange" };
    case "detected":
      return { text: "Detected", cls: "badge-blue" };
    default:
      return { text: status, cls: "badge-muted" };
  }
}

async function loadView() {
  if (!slug.value) return;
  loading.value = true;
  error.value = "";
  try {
    view.value = await api.getStatusPageView(slug.value);
  } catch (e: unknown) {
    view.value = null;
    error.value = getErrorMessage(e, "Failed to load status page");
  } finally {
    loading.value = false;
  }
}

async function changeComponentStatus(c: StatusPageComponentRecord, status: ComponentStatus) {
  if (!view.value || c.status === status) return;
  try {
    await api.updateStatusPageComponent(view.value.page.id, c.id, { status });
    c.status = status;
    // Recompute overall from the worst component status.
    view.value.overall_status = computeOverall(view.value.components);
    push("Component status updated", "success");
  } catch (e: unknown) {
    push(getErrorMessage(e, "Failed to update component"), "error");
  }
}

function computeOverall(components: StatusPageComponentRecord[]): ComponentStatus {
  const rank: Record<ComponentStatus, number> = {
    operational: 0,
    maintenance: 1,
    degraded: 2,
    partial_outage: 3,
    major_outage: 4,
  };
  let worst: ComponentStatus = "operational";
  for (const c of components) {
    if ((rank[c.status] ?? 0) > (rank[worst] ?? 0)) worst = c.status;
  }
  return worst;
}

async function addComponent() {
  if (!view.value || !newComponentName.value.trim()) return;
  adding.value = true;
  try {
    const created = await api.createStatusPageComponent(view.value.page.id, {
      name: newComponentName.value.trim(),
      status: newComponentStatus.value,
      display_order: view.value.components.length,
    });
    view.value.components.push(created);
    view.value.overall_status = computeOverall(view.value.components);
    newComponentName.value = "";
    newComponentStatus.value = "operational";
    push("Component added", "success");
  } catch (e: unknown) {
    push(getErrorMessage(e, "Failed to add component"), "error");
  } finally {
    adding.value = false;
  }
}

function requestRemoveComponent(c: StatusPageComponentRecord) {
  removeTarget.value = c;
}

async function confirmRemoveComponent() {
  const target = removeTarget.value;
  if (!view.value || !target) return;
  removing.value = true;
  try {
    await api.deleteStatusPageComponent(view.value.page.id, target.id);
    view.value.components = view.value.components.filter((x) => x.id !== target.id);
    view.value.overall_status = computeOverall(view.value.components);
    push("Component removed", "success");
    removeTarget.value = null;
  } catch (e: unknown) {
    push(getErrorMessage(e, "Failed to remove component"), "error");
  } finally {
    removing.value = false;
  }
}

function cancelRemoveComponent() {
  if (removing.value) return;
  removeTarget.value = null;
}

onMounted(loadView);
watch(slug, loadView);
</script>

<template>
  <section class="mx-auto max-w-3xl space-y-5 px-4 py-6 md:py-8">
    <Button
      type="button"
      variant="outline"
      size="sm"
      class="flex items-center gap-1 text-[var(--text-muted)] hover:text-[var(--text-primary)]"
      @click="router.push('/settings/status-pages')"
    >
      <ArrowLeft class="h-4 w-4" /> Back to Status Pages
    </Button>

    <LoadingSpinner v-if="loading && !view" />
    <ErrorBanner v-else-if="error" :message="error" @dismiss="error = ''" />

    <template v-else-if="view">
      <header class="space-y-1">
        <div class="flex items-center gap-2">
          <h1 class="text-2xl font-semibold">{{ view.page.name }}</h1>
          <button
            class="text-[var(--text-muted)] hover:text-[var(--text-primary)]"
            title="Refresh"
            @click="loadView"
          >
            <RefreshCw class="h-4 w-4" />
          </button>
        </div>
        <p v-if="view.page.description" class="text-sm text-[var(--text-secondary)]">
          {{ view.page.description }}
        </p>
      </header>

      <div :class="['rounded-lg border p-4', statusMeta(view.overall_status).banner]">
        <div class="flex items-center gap-3">
          <span :class="['h-3 w-3 rounded-full', statusMeta(view.overall_status).dot]" />
          <span class="text-lg font-medium">{{ statusMeta(view.overall_status).label }}</span>
        </div>
      </div>

      <div v-if="view.incidents.length > 0" class="space-y-2">
        <h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--text-muted)]">
          Active Incidents
        </h2>
        <Card v-for="inc in view.incidents" :key="inc.id">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="min-w-0 flex-1 space-y-1">
              <div class="flex flex-wrap items-center gap-2">
                <span :class="['badge', incidentStatusLabel(inc.status).cls]">
                  {{ incidentStatusLabel(inc.status).text }}
                </span>
                <span class="badge badge-muted">{{ inc.severity }}</span>
                <span class="font-medium">{{
                  inc.title || `Incident #${inc.incident_number}`
                }}</span>
              </div>
              <div class="text-xs text-[var(--text-muted)]">
                Started {{ formatTimeAgo(inc.started_at || inc.created_at) }}
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              @click="router.push(`/incidents/${inc.incident_number}`)"
            >
              Open
            </Button>
          </div>
        </Card>
      </div>

      <div class="space-y-2">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--text-muted)]">
            Components
          </h2>
        </div>

        <Card v-for="c in view.components" :key="c.id">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="min-w-0 flex-1 space-y-1">
              <div class="flex flex-wrap items-center gap-2">
                <span :class="['h-2.5 w-2.5 rounded-full', statusMeta(c.status).dot]" />
                <span class="font-medium">{{ c.name }}</span>
                <span :class="['badge', statusMeta(c.status).badge]">{{
                  c.status.replace("_", " ")
                }}</span>
              </div>
              <p v-if="c.description" class="text-sm text-[var(--text-secondary)]">
                {{ c.description }}
              </p>
            </div>
            <div class="flex items-center gap-2">
              <Select
                v-if="canWrite"
                :modelValue="c.status"
                class="rounded-md p-2 text-sm"
                @update:modelValue="(v: string) => changeComponentStatus(c, v as ComponentStatus)"
              >
                <option v-for="s in COMPONENT_STATUS_OPTIONS" :key="s" :value="s">
                  {{ s.replace("_", " ") }}
                </option>
              </Select>
              <Button
                v-if="canDelete"
                variant="outline"
                size="sm"
                @click="requestRemoveComponent(c)"
              >
                <Trash2 class="h-4 w-4" />
              </Button>
            </div>
          </div>
        </Card>

        <EmptyState v-if="view.components.length === 0" message="No components yet." />

        <div v-if="canWrite" class="flex flex-wrap items-end gap-2 pt-2">
          <label class="flex flex-col gap-1 text-xs font-medium text-[var(--text-muted)]">
            New component
            <Input
              v-model="newComponentName"
              type="text"
              class="w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] p-2 text-sm"
              placeholder="e.g. API"
              @keydown.enter="addComponent"
            />
          </label>
          <label class="flex flex-col gap-1 text-xs font-medium text-[var(--text-muted)]">
            Status
            <Select v-model="newComponentStatus" class="rounded-md p-2 text-sm">
              <option v-for="s in COMPONENT_STATUS_OPTIONS" :key="s" :value="s">
                {{ s.replace("_", " ") }}
              </option>
            </Select>
          </label>
          <Button size="sm" :disabled="adding || !newComponentName.trim()" @click="addComponent">
            <Plus class="mr-1 h-3.5 w-3.5" /> Add
          </Button>
        </div>
      </div>

      <p class="text-xs text-[var(--text-muted)]">
        Last updated {{ formatTime(view.page.updated_at) }}
      </p>
    </template>

    <ConfirmDialog
      :open="removeTarget !== null"
      :title="removeTarget ? `Delete component ${removeTarget.name}?` : 'Delete component?'"
      :message="
        removeTarget
          ? `This permanently removes ${removeTarget.name} from the status page.`
          : 'This permanently removes the selected component from the status page.'
      "
      destructive
      :loading="removing"
      confirm-label="Delete"
      @confirm="confirmRemoveComponent"
      @cancel="cancelRemoveComponent"
    />
  </section>
</template>
