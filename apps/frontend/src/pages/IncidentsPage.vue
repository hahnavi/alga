<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import {
  computed,
  onActivated,
  onBeforeUnmount,
  onDeactivated,
  onMounted,
  reactive,
  ref,
  watch,
} from "vue";
import { useRoute, useRouter } from "vue-router";
import { Loader2, AlertTriangle, X } from "@lucide/vue";
import {
  api,
  type IncidentRecord,
  type IncidentResolveResponse,
  type Severity,
  type ImpactLevel,
  type IncidentPriority,
} from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import Textarea from "@/components/ui/Textarea.vue";
import Select from "@/components/ui/Select.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import Card from "@/components/ui/Card.vue";
import IncidentCard from "@/components/incident/IncidentCard.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import SortSelect from "@/components/ui/SortSelect.vue";
import Modal from "@/components/ui/Modal.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import type { SortOption } from "@/components/ui/SortSelect.vue";
import { INCIDENT_PRIORITY_TABS, INCIDENT_STATUS_TABS } from "@/lib/alertLabels";
import { useToast } from "@/lib/toast";
import { useFilterSync } from "@/composables/useFilterSync";
import { useSearchDebounce } from "@/composables/useSearchDebounce";
import { useFormSubmit } from "@/composables/useFormSubmit";
import { useScrollRestore } from "@/composables/useScrollRestore";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useDelete } from "@/composables/useDelete";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "IncidentsPage" });

const router = useRouter();
const route = useRoute();
const { push } = useToast();
const { scheduleSearchReload } = useSearchDebounce(() => loadIncidents(), 400);

const SORT_OPTIONS: SortOption[] = [
  { label: "Recently updated", value: "updated_at" },
  { label: "Created time", value: "created_at" },
  { label: "Severity", value: "severity" },
  { label: "Status", value: "status" },
];

const DEFAULT_SORT = "updated_at";

const incidents = ref<IncidentRecord[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");
const statusFilter = ref("all");
const priorityFilter = ref("all");
const searchInput = ref("");
const sortBy = ref(DEFAULT_SORT);
const skip = ref(0);
const pageSize = 50;
const navigatingId = ref<number | null>(null);

const { canCommand, canDelete, canWrite } = useEntityPermissions("incidents");
const actionLoadingMap = reactive<Record<number, boolean>>({});

function passesFilters(inc: IncidentRecord): boolean {
  if (statusFilter.value !== "all" && inc.status !== statusFilter.value) return false;
  if (priorityFilter.value !== "all" && inc.priority !== priorityFilter.value) return false;
  return true;
}

async function runIncidentAction(
  inc: IncidentRecord,
  fn: (n: number) => Promise<IncidentRecord | IncidentResolveResponse>,
  successMsg: string,
) {
  const n = inc.incident_number;
  if (actionLoadingMap[n]) return;
  actionLoadingMap[n] = true;
  try {
    const res = await fn(n);
    const updated = "incident" in res ? res.incident : res;
    const idx = incidents.value.findIndex((i) => i.incident_number === n);
    if (idx !== -1) {
      if (passesFilters(updated)) {
        incidents.value.splice(idx, 1, updated);
      } else {
        incidents.value.splice(idx, 1);
      }
    }
    push(successMsg, "success");
  } catch (err) {
    push(getErrorMessage(err, "Action failed"), "error");
  } finally {
    delete actionLoadingMap[n];
  }
}

function resolveIncident(inc: IncidentRecord) {
  void runIncidentAction(inc, (n) => api.resolveIncident(n), "Incident resolved");
}
function closeIncident(inc: IncidentRecord) {
  void runIncidentAction(inc, (n) => api.closeIncident(n), "Incident closed");
}
function reopenIncident(inc: IncidentRecord) {
  void runIncidentAction(inc, (n) => api.reopenIncident(n), "Incident reopened");
}

const { showDeleteConfirm, confirmDelete, doDelete } = useDelete<IncidentRecord>(async (inc) => {
  await api.deleteIncident(inc.incident_number);
  incidents.value = incidents.value.filter((i) => i.incident_number !== inc.incident_number);
}, "Incident");

const showCreateDialog = ref(false);
const createTitle = ref("");
const createDescription = ref("");
const createSeverity = ref<Severity | "">("");
const createImpact = ref<ImpactLevel | "">("");
const createServiceId = ref("");
const { submitting: createSubmitting, formError: createError, withSubmit } = useFormSubmit();
const createServiceSearch = ref("");
const showServiceDropdown = ref(false);
const serviceOptions = ref<{ id: string; name: string; display_name: string; status: string }[]>(
  [],
);
let serviceSearchTimeout: number | null = null;
let serviceBlurTimeout: number | null = null;

function openCreateDialog() {
  createTitle.value = "";
  createDescription.value = "";
  createSeverity.value = "";
  createImpact.value = "";
  createServiceId.value = "";
  createServiceSearch.value = "";
  showServiceDropdown.value = false;
  serviceOptions.value = [];
  createError.value = "";
  createSubmitting.value = false;
  showCreateDialog.value = true;
}

function closeCreateDialog() {
  if (createSubmitting.value) return;
  showCreateDialog.value = false;
}

function onServiceSearchInput(q: string) {
  createServiceSearch.value = q;
  createServiceId.value = "";
  if (serviceSearchTimeout) clearTimeout(serviceSearchTimeout);
  if (!q.trim()) {
    serviceOptions.value = [];
    showServiceDropdown.value = false;
    return;
  }
  serviceSearchTimeout = setTimeout(async () => {
    try {
      const res = await api.getServices({ q: q.trim(), limit: 10 });
      serviceOptions.value = res.items.map((s) => ({
        id: s.id,
        name: s.name,
        display_name: s.display_name,
        status: s.status,
      }));
      showServiceDropdown.value = serviceOptions.value.length > 0;
    } catch {
      serviceOptions.value = [];
    }
  }, 300);
}

function selectService(svc: { id: string; name: string; display_name: string }) {
  createServiceId.value = svc.id;
  createServiceSearch.value = svc.display_name || svc.name;
  showServiceDropdown.value = false;
}

function onServiceBlur() {
  if (serviceBlurTimeout) clearTimeout(serviceBlurTimeout);
  serviceBlurTimeout = setTimeout(() => {
    showServiceDropdown.value = false;
  }, 150);
}

const filteredServices = computed(() => serviceOptions.value);

const priorityMatrix: Record<string, Record<string, IncidentPriority>> = {
  critical: { high: "P1", medium: "P2", low: "P3" },
  high: { high: "P2", medium: "P3", low: "P4" },
  warning: { high: "P3", medium: "P4", low: "P5" },
  info: { high: "P4", medium: "P5", low: "P5" },
};

const computedPriority = computed<IncidentPriority | "">(() => {
  if (!createSeverity.value || !createImpact.value) return "";
  return priorityMatrix[createSeverity.value]?.[createImpact.value] ?? "";
});

async function submitCreateIncident() {
  if (createSubmitting.value) return;
  const title = createTitle.value.trim();
  if (!title) {
    createError.value = "Title is required.";
    return;
  }
  await withSubmit(async () => {
    const input: Parameters<typeof api.createIncident>[0] = { title };
    const desc = createDescription.value.trim();
    if (desc) input.description = desc;
    if (createSeverity.value) input.severity = createSeverity.value;
    if (createImpact.value) input.impact_level = createImpact.value;
    const priority = computedPriority.value;
    if (priority) input.priority = priority;
    const sid = createServiceId.value.trim();
    if (sid) input.service_id = sid;
    await api.createIncident(input);
    showCreateDialog.value = false;
    lastLoadedQuery = "";
    loadIncidents();
  }, "Incident created");
}

onBeforeUnmount(() => {
  if (serviceSearchTimeout) clearTimeout(serviceSearchTimeout);
  if (serviceBlurTimeout) clearTimeout(serviceBlurTimeout);
});

const statusTabs = INCIDENT_STATUS_TABS;
const priorityTabs = INCIDENT_PRIORITY_TABS;

const hasNonDefaultFilters = computed(
  () =>
    statusFilter.value !== "all" ||
    priorityFilter.value !== "all" ||
    searchInput.value.trim() !== "" ||
    sortBy.value !== DEFAULT_SORT,
);

const { showFilters } = usePageHeaderActions({
  title: "Incidents",
  titleIcon: AlertTriangle,
  searchInput,
  searchPlaceholder: "Search incidents...",
  onSearchInput: scheduleSearchReload,
  showFilters: true,
  hasNonDefaultFilters,
  onAdd: openCreateDialog,
  addLabel: "Create incident",
  showAdd: canWrite,
});

function buildFilterParams(): Record<string, string> {
  const out: Record<string, string> = {};
  if (statusFilter.value !== "all") out.status = statusFilter.value;
  if (priorityFilter.value !== "all") out.priority = priorityFilter.value;
  if (searchInput.value.trim()) out.search = searchInput.value.trim();
  if (sortBy.value && sortBy.value !== DEFAULT_SORT) out.sort = sortBy.value;
  return out;
}

const filterSync = useFilterSync({
  route,
  router,
  path: "/incidents",
  buildQuery: buildFilterParams,
  parseQuery: (q) => {
    const status = typeof q.status === "string" ? q.status : "";
    const priority = typeof q.priority === "string" ? q.priority : "";
    const search = typeof q.search === "string" ? q.search : "";
    const sortRaw = typeof q.sort === "string" ? q.sort : "";
    statusFilter.value = statusTabs.some((tab) => tab.value === status) ? status : "all";
    priorityFilter.value = priorityTabs.some((tab) => tab.value === priority) ? priority : "all";
    searchInput.value = search;
    sortBy.value =
      sortRaw && SORT_OPTIONS.some((o) => sortRaw.endsWith(o.value) || sortRaw === o.value)
        ? sortRaw
        : DEFAULT_SORT;
  },
  onReload: () => loadIncidents(),
});

function clearFilters() {
  filterSync.clearFilters(() => {
    statusFilter.value = "all";
    priorityFilter.value = "all";
    searchInput.value = "";
    sortBy.value = DEFAULT_SORT;
  });
}

const statusLabel = computed(() => {
  const tab = statusTabs.find((t) => t.value === statusFilter.value);
  return tab?.label ?? "All statuses";
});

const priorityLabel = computed(() => {
  const tab = priorityTabs.find((t) => t.value === priorityFilter.value);
  return tab?.label ?? "All priorities";
});

const hasMore = () => incidents.value.length < total.value;

let lastLoadedQuery = "";

async function loadIncidents(append = false) {
  if (!append) {
    const currentQuery = JSON.stringify(buildFilterParams());
    if (currentQuery === lastLoadedQuery && incidents.value.length > 0) return;
    lastLoadedQuery = currentQuery;
  }
  loading.value = true;
  error.value = "";
  if (!append) {
    skip.value = 0;
  }
  try {
    const data = await api.getIncidents({
      status: statusFilter.value !== "all" ? statusFilter.value : undefined,
      priority: priorityFilter.value !== "all" ? priorityFilter.value : undefined,
      search: searchInput.value || undefined,
      sort: sortBy.value || undefined,
      limit: pageSize,
      skip: skip.value,
    });
    if (append) {
      incidents.value = [...incidents.value, ...(data.items || [])];
    } else {
      incidents.value = data.items || [];
    }
    total.value = data.total ?? 0;
  } catch (err) {
    const msg = getErrorMessage(err, "Failed to load incidents");
    error.value = msg;
    push(msg, "error");
  } finally {
    loading.value = false;
  }
}

function loadMore() {
  skip.value += pageSize;
  loadIncidents(true);
}

function goToIncident(incidentNumber: number) {
  navigatingId.value = incidentNumber;
  router.push(`/incidents/${incidentNumber}`);
}

useScrollRestore({ skipFirst: true });
let isDeactivated = false;

onMounted(() => {
  filterSync.applyFromUrl();
  loadIncidents();
});

onActivated(() => {
  isDeactivated = false;
  navigatingId.value = null;
  lastLoadedQuery = "";
  loadIncidents();
});

onDeactivated(() => {
  isDeactivated = true;
  lastLoadedQuery = "";
});

watch(statusFilter, () => {
  if (isDeactivated) return;
  filterSync.syncFiltersToUrl();
  loadIncidents();
});

watch(priorityFilter, () => {
  if (isDeactivated) return;
  filterSync.syncFiltersToUrl();
  loadIncidents();
});

watch(sortBy, () => {
  if (isDeactivated) return;
  filterSync.syncFiltersToUrl();
  loadIncidents();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <Card v-if="showFilters" class="!p-4">
      <div class="flex flex-wrap items-end gap-3">
        <div class="min-w-0 w-full sm:w-auto sm:min-w-[12rem]">
          <FormLabel for="incidents-status-filter">Status</FormLabel>
          <Select
            id="incidents-status-filter"
            v-model="statusFilter"
            class="w-full"
            aria-label="Filter by incident status"
          >
            <option v-for="tab in statusTabs" :key="tab.value" :value="tab.value">
              {{ tab.label }}
            </option>
          </Select>
        </div>
        <div class="min-w-0 w-full sm:w-auto sm:min-w-[12rem]">
          <FormLabel for="incidents-priority-filter">Priority</FormLabel>
          <Select
            id="incidents-priority-filter"
            v-model="priorityFilter"
            class="w-full"
            aria-label="Filter by incident priority"
          >
            <option v-for="tab in priorityTabs" :key="tab.value" :value="tab.value">
              {{ tab.label }}
            </option>
          </Select>
        </div>
        <div class="min-w-0 w-full sm:w-auto sm:min-w-[12rem]">
          <FormLabel for="incidents-sort-filter">Sort</FormLabel>
          <SortSelect
            id="incidents-sort-filter"
            v-model="sortBy"
            :options="SORT_OPTIONS"
            :default-sort="DEFAULT_SORT"
            class="w-full"
          />
        </div>
        <Button
          v-if="hasNonDefaultFilters"
          variant="outline"
          size="sm"
          type="button"
          @click="clearFilters"
        >
          <X class="h-3.5 w-3.5" aria-hidden="true" />
          Clear filters
        </Button>
      </div>
    </Card>
    <p class="text-xs text-[var(--text-muted)]">
      Showing: {{ statusLabel }}
      <template v-if="priorityFilter !== 'all'"> · {{ priorityLabel }} </template>
      · {{ total }} incident{{ total !== 1 ? "s" : "" }}
    </p>
    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading && incidents.length === 0" centered />
    <EmptyState v-else-if="incidents.length === 0" message="No incidents found.">
      <template #icon>
        <AlertTriangle class="mb-2 h-6 w-6 opacity-40" />
      </template>
      <template
        v-if="statusFilter !== 'all' || priorityFilter !== 'all' || searchInput.trim() !== ''"
        #footer
      >
        <p class="mt-1 text-xs text-[var(--text-muted)]">
          Try adjusting your filters or
          <button
            type="button"
            class="text-[var(--text-link)] hover:underline"
            @click="clearFilters"
          >
            clear all filters</button
          >.
        </p>
      </template>
    </EmptyState>
    <div v-else class="space-y-3">
      <IncidentCard
        v-for="inc in incidents"
        :key="inc.incident_number"
        :incident="inc"
        :loading="navigatingId === inc.incident_number"
        :can-command="canCommand"
        :can-delete="canDelete"
        :status-busy="!!actionLoadingMap[inc.incident_number]"
        @navigate="goToIncident(inc.incident_number)"
        @resolve="resolveIncident(inc)"
        @close="closeIncident(inc)"
        @reopen="reopenIncident(inc)"
        @delete="confirmDelete(inc)"
      />

      <div v-if="hasMore()" class="flex justify-center pt-2">
        <Button variant="outline" :disabled="loading" @click="loadMore">
          <Loader2 v-if="loading" class="h-4 w-4 animate-spin" />
          Load More
        </Button>
      </div>
    </div>

    <Modal
      :open="showCreateDialog"
      title="Create incident"
      max-width="xl"
      :prevent-close="createSubmitting"
      @update:open="!$event && closeCreateDialog()"
      @close="closeCreateDialog"
    >
      <form class="space-y-4" @submit.prevent="submitCreateIncident">
        <ErrorBanner :message="createError" />
        <div>
          <FormLabel for="create-incident-title-input" required>Title</FormLabel>
          <Input
            id="create-incident-title-input"
            v-model="createTitle"
            required
            autocomplete="off"
            placeholder="Brief incident title"
            :disabled="createSubmitting"
            aria-required="true"
          />
        </div>
        <div>
          <FormLabel for="create-incident-desc">Description</FormLabel>
          <Textarea
            id="create-incident-desc"
            v-model="createDescription"
            rows="3"
            class="min-h-[4.5rem] w-full resize-y"
            placeholder="What happened?"
            :disabled="createSubmitting"
          />
        </div>
        <div>
          <FormLabel for="create-incident-severity">Severity</FormLabel>
          <Select
            id="create-incident-severity"
            v-model="createSeverity"
            class="w-full"
            :disabled="createSubmitting"
            aria-label="Severity"
          >
            <option value="">No severity</option>
            <option value="critical">Critical</option>
            <option value="high">High</option>
            <option value="warning">Warning</option>
            <option value="info">Info</option>
          </Select>
        </div>
        <div>
          <FormLabel for="create-incident-impact">Impact</FormLabel>
          <Select
            id="create-incident-impact"
            v-model="createImpact"
            class="w-full"
            :disabled="createSubmitting"
            aria-label="Impact"
          >
            <option value="">No impact</option>
            <option value="high">High</option>
            <option value="medium">Medium</option>
            <option value="low">Low</option>
          </Select>
        </div>
        <div v-if="computedPriority">
          <FormLabel>Priority (computed)</FormLabel>
          <p class="text-sm font-medium text-[var(--text-primary)]">{{ computedPriority }}</p>
        </div>
        <div>
          <FormLabel for="create-incident-service">Service</FormLabel>
          <div class="relative">
            <Input
              id="create-incident-service"
              type="text"
              autocomplete="off"
              placeholder="Search services..."
              :disabled="createSubmitting"
              :model-value="createServiceSearch"
              class="w-full"
              @update:model-value="onServiceSearchInput"
              @focus="showServiceDropdown = true"
              @blur="onServiceBlur"
            />
            <ul
              v-if="showServiceDropdown && filteredServices.length > 0"
              class="absolute left-0 right-0 top-full z-10 mt-1 max-h-48 overflow-y-auto rounded border border-[var(--border-primary)] bg-[var(--bg-card)] shadow-lg"
            >
              <li
                v-for="svc in filteredServices"
                :key="svc.id"
                class="cursor-pointer px-3 py-2 text-sm text-[var(--text-primary)] hover:bg-[var(--bg-secondary)]"
                @mousedown.prevent="selectService(svc)"
              >
                {{ svc.display_name || svc.name }}
                <span class="text-xs text-[var(--text-muted)]">{{ svc.status }}</span>
              </li>
            </ul>
            <p v-if="!createServiceId" class="mt-1 text-xs text-[var(--text-muted)]">
              Recommended for auto-assignment of incident commander and responders.
            </p>
          </div>
        </div>
      </form>
      <template #footer>
        <Button variant="outline" :disabled="createSubmitting" @click="closeCreateDialog"
          >Cancel</Button
        >
        <Button :disabled="createSubmitting" @click="submitCreateIncident">
          {{ createSubmitting ? "Creating…" : "Create incident" }}
        </Button>
      </template>
    </Modal>

    <ConfirmDialog
      :open="showDeleteConfirm"
      title="Delete incident"
      message="Are you sure you want to delete this incident? This cannot be undone."
      confirm-label="Delete"
      destructive
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />
  </section>
</template>
