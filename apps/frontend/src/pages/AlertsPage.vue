<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import {
  computed,
  nextTick,
  onActivated,
  onDeactivated,
  onMounted,
  onUnmounted,
  reactive,
  ref,
  watch,
} from "vue";
import { useRoute, useRouter, type LocationQuery } from "vue-router";
import { queryString } from "@/lib/routing";
import { Bell, Bot, CircleAlert, Clock, User, X, ShieldCheck } from "@lucide/vue";
import { api, type AlertRecord } from "@/lib/api";
import { alertRecordSchema, validate } from "@/lib/validation";
import {
  alertSeverityLabel,
  nonHeaderLabelKeys,
  severityRailClass,
  sortAlerts,
} from "@/lib/alertLabels";
import { formatTimeFull } from "@/lib/time";
import Button from "@/components/ui/Button.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Select from "@/components/ui/Select.vue";
import Card from "@/components/ui/Card.vue";
import CreateAlertDialog from "@/components/CreateAlertDialog.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import SkeletonRows from "@/components/ui/SkeletonRows.vue";
import InteractiveCard from "@/components/ui/InteractiveCard.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import SortSelect from "@/components/ui/SortSelect.vue";
import AlertActionsMenu from "@/components/ui/AlertActionsMenu.vue";
import AlertStatusBadge from "@/components/ui/AlertStatusBadge.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import DateTimePicker from "@/components/ui/DateTimePicker.vue";
import type { SortOption } from "@/components/ui/SortSelect.vue";
import { useSSE } from "@/composables/useSSE";
import { useScrollRestore } from "@/composables/useScrollRestore";
import { useFilterSync } from "@/composables/useFilterSync";
import { useSearchDebounce } from "@/composables/useSearchDebounce";
import { useDelete } from "@/composables/useDelete";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { useInfiniteScroll } from "@/composables/useInfiniteScroll";
import { useToast } from "@/lib/toast";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";

defineOptions({ name: "AlertsPage" });

const PAGE_SIZE = 15;
const MAX_VISIBLE_LABELS = 5;

const ALERT_SORT_OPTIONS: SortOption[] = [
  { label: "Recently updated", value: "updated_at" },
  { label: "Created time", value: "created_at" },
  { label: "Severity", value: "severity" },
  { label: "Status", value: "status" },
];

const DEFAULT_ALERT_SORT = "-updated_at";

const route = useRoute();
const router = useRouter();

function isValidDatetimeLocal(v: string): boolean {
  if (!v) return false;
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(v);
}

function parseAlertsQuery(q: LocationQuery) {
  const st = queryString(q, "status");
  const statusVal = ["all", "open", "closed", "unacknowledged", "acknowledged"].includes(st)
    ? st
    : "all";
  const rangeRaw = queryString(q, "range");
  const rangeVal = ["all", "1h", "24h", "7d", "30d", "1y", "custom"].includes(rangeRaw)
    ? rangeRaw
    : "all";
  const from = queryString(q, "from");
  const to = queryString(q, "to");
  const searchQ = queryString(q, "q");
  const sortRaw = queryString(q, "sort");
  const sortVal =
    sortRaw && ALERT_SORT_OPTIONS.some((o) => sortRaw.endsWith(o.value) || sortRaw === o.value)
      ? sortRaw
      : DEFAULT_ALERT_SORT;
  return {
    status: statusVal,
    timeRange: rangeVal,
    customStart: isValidDatetimeLocal(from) ? from : "",
    customEnd: isValidDatetimeLocal(to) ? to : "",
    searchInput: searchQ,
    sort: sortVal,
  };
}

const initFilters = parseAlertsQuery(route.query);

const alerts = ref<AlertRecord[]>([]);
const loading = ref(false);
const loadMoreLoading = ref(false);
const hasMore = ref(true);
const error = ref("");
const status = ref(initFilters.status);
const timeRange = ref(initFilters.timeRange);
const customStart = ref(initFilters.customStart);
const customEnd = ref(initFilters.customEnd);
const showCustomRange = ref(false);
const searchInput = ref(initFilters.searchInput);
const sortBy = ref(initFilters.sort);
const showCreateDialog = ref(false);
const navigatingAlertNumber = ref<string | null>(null);

const hasNonDefaultFilters = computed(
  () =>
    status.value !== "all" ||
    timeRange.value !== "all" ||
    searchInput.value.trim() !== "" ||
    sortBy.value !== DEFAULT_ALERT_SORT,
);

function navigateToAlert(alertNumber: number) {
  navigatingAlertNumber.value = String(alertNumber);
  router.push(`/alerts/${alertNumber}`);
}
const { push } = useToast();
const { canWrite, canDelete } = useEntityPermissions("alerts");

const ackLoadingMap = reactive<Record<number, boolean>>({});
const resolveLoadingMap = reactive<Record<number, boolean>>({});

async function acknowledgeAlert(alert: AlertRecord) {
  if (alert.alert_number == null || ackLoadingMap[alert.alert_number]) return;
  ackLoadingMap[alert.alert_number] = true;
  try {
    const updated = await api.acknowledgeAlert(alert.alert_number);
    const idx = alerts.value.findIndex((a) => a.alert_number === alert.alert_number);
    if (idx !== -1) {
      if (passesListFilters(updated)) {
        alerts.value.splice(idx, 1, updated);
      } else {
        alerts.value.splice(idx, 1);
      }
    }
    push("Alert acknowledged", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to acknowledge alert"), "error");
  } finally {
    if (alert.alert_number != null) delete ackLoadingMap[alert.alert_number];
  }
}

async function resolveAlert(alert: AlertRecord) {
  if (alert.alert_number == null || resolveLoadingMap[alert.alert_number]) return;
  resolveLoadingMap[alert.alert_number] = true;
  try {
    const updated = await api.resolveAlert(alert.alert_number);
    const idx = alerts.value.findIndex((a) => a.alert_number === alert.alert_number);
    if (idx !== -1) {
      if (passesListFilters(updated)) {
        alerts.value.splice(idx, 1, updated);
      } else {
        alerts.value.splice(idx, 1);
      }
    }
    push("Alert resolved", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to resolve alert"), "error");
  } finally {
    if (alert.alert_number != null) delete resolveLoadingMap[alert.alert_number];
  }
}

const { showDeleteConfirm, confirmDelete, doDelete } = useDelete<AlertRecord>(async (alert) => {
  if (alert.alert_number == null) return;
  await api.deleteAlert(alert.alert_number);
  alerts.value = alerts.value.filter((a) => a.alert_number !== alert.alert_number);
}, "Alert");

const sseHighlights = reactive<Record<string, boolean>>({});
const sseHighlightClearTimers = new Map<string, number>();
const SSE_APPEND_HIGHLIGHT_MS = 2600;

function markSseAppendedHighlight(fingerprint: string) {
  const prev = sseHighlightClearTimers.get(fingerprint);
  if (prev != null) clearTimeout(prev);
  sseHighlights[fingerprint] = true;
  const t = setTimeout(() => {
    sseHighlightClearTimers.delete(fingerprint);
    delete sseHighlights[fingerprint];
  }, SSE_APPEND_HIGHLIGHT_MS);
  sseHighlightClearTimers.set(fingerprint, t);
}

function isSseAppendedHighlight(fingerprint: string): boolean {
  return !!sseHighlights[fingerprint];
}

function buildFilterParams(): Record<string, string> {
  const out: Record<string, string> = {};
  if (status.value !== "all") out.status = status.value;
  if (timeRange.value !== "all") {
    out.range = timeRange.value;
    if (timeRange.value === "custom" && customStart.value && customEnd.value) {
      out.from = customStart.value;
      out.to = customEnd.value;
    }
  }
  const sq = searchInput.value.trim();
  if (sq) out.q = sq;
  if (sortBy.value && sortBy.value !== DEFAULT_ALERT_SORT) out.sort = sortBy.value;
  return out;
}

const filterSync = useFilterSync({
  route,
  router,
  path: "/alerts",
  buildQuery: buildFilterParams,
  parseQuery: (q) => {
    const f = parseAlertsQuery(q);
    status.value = f.status;
    timeRange.value = f.timeRange;
    customStart.value = f.customStart;
    customEnd.value = f.customEnd;
    searchInput.value = f.searchInput;
    sortBy.value = f.sort;
  },
  onReload: () => load(true),
});

function matchesStatusFilter(alert: AlertRecord): boolean {
  switch (status.value) {
    case "open":
      return alert.status === "firing";
    case "closed":
      return alert.status === "resolved";
    case "unacknowledged":
      return alert.status === "firing" && !alert.acknowledged;
    case "acknowledged":
      return !!alert.acknowledged;
    default:
      return true;
  }
}

function matchesSearch(alert: AlertRecord): boolean {
  const raw = searchInput.value.trim();
  if (!raw) return true;
  const numMatch = /^#?(\d+)$/.exec(raw);
  if (numMatch) {
    const n = Number(numMatch[1]);
    return alert.alert_number === n;
  }
  const q = raw.toLowerCase();
  const name = (alert.labels?.alertname ?? "").toLowerCase();
  return name.includes(q);
}

function matchesTimeRange(alert: AlertRecord): boolean {
  if (timeRange.value === "all") return true;
  if (timeRange.value === "custom") {
    if (!customStart.value || !customEnd.value) return true;
    const t = new Date(alert.updated_at).getTime();
    return t >= new Date(customStart.value).getTime() && t <= new Date(customEnd.value).getTime();
  }
  const range = resolveTimeRange();
  if (!range.startDate || !range.endDate) return true;
  const t = new Date(alert.updated_at).getTime();
  return t >= new Date(range.startDate).getTime() && t <= new Date(range.endDate).getTime();
}

function passesListFilters(alert: AlertRecord): boolean {
  return matchesStatusFilter(alert) && matchesSearch(alert) && matchesTimeRange(alert);
}

function allLabelKeys(alert: AlertRecord): string[] {
  return nonHeaderLabelKeys(alert.labels);
}

function limitedLabelKeys(alert: AlertRecord): string[] {
  return allLabelKeys(alert).slice(0, MAX_VISIBLE_LABELS);
}

function hiddenLabelCount(alert: AlertRecord): number {
  const total = allLabelKeys(alert).length;
  return total > MAX_VISIBLE_LABELS ? total - MAX_VISIBLE_LABELS : 0;
}

function investigationActor(alert: AlertRecord): { name: string; isAgent: boolean } | null {
  const inv = alert.investigation;
  if (!inv) return null;
  const name = inv.agent_name?.trim();
  if (!name) return null;
  const isAgent = inv.assignee_type !== "user";
  return { name, isAgent };
}

function handleAlertCreatedSSE(data: unknown) {
  let alert: AlertRecord;
  try {
    alert = validate(alertRecordSchema, data) as AlertRecord;
  } catch {
    return; // malformed event — drop instead of corrupting the list
  }
  if (!passesListFilters(alert)) return;
  if (alerts.value.some((a) => a.alert_number === alert.alert_number)) return;
  alerts.value = sortAlerts([alert, ...alerts.value], sortBy.value);
  markSseAppendedHighlight(alert.fingerprint);
}

function handleAlertUpdatedSSE(data: unknown) {
  let alert: AlertRecord;
  try {
    alert = validate(alertRecordSchema, data) as AlertRecord;
  } catch {
    return;
  }
  const idx = alerts.value.findIndex((a) => a.alert_number === alert.alert_number);
  if (idx !== -1) {
    if (passesListFilters(alert)) {
      alerts.value.splice(idx, 1, alert);
      alerts.value = sortAlerts(alerts.value, sortBy.value);
    } else {
      alerts.value.splice(idx, 1);
    }
  } else if (passesListFilters(alert)) {
    if (!alerts.value.some((a) => a.alert_number === alert.alert_number)) {
      alerts.value = sortAlerts([alert, ...alerts.value], sortBy.value);
      markSseAppendedHighlight(alert.fingerprint);
    }
  }
}

function handleAlertDeletedSSE(data: unknown) {
  let alert: AlertRecord;
  try {
    alert = validate(alertRecordSchema, data) as AlertRecord;
  } catch {
    return;
  }
  alerts.value = alerts.value.filter((a) => a.alert_number !== alert.alert_number);
}

const INVESTIGATION_REFRESH_DEBOUNCE_MS = 1500;
let investigationRefreshTimer: number | null = null;

function scheduleInvestigationRefresh() {
  if (investigationRefreshTimer != null) clearTimeout(investigationRefreshTimer);
  investigationRefreshTimer = setTimeout(() => {
    investigationRefreshTimer = null;
    lastLoadedQuery = "";
    void load(true);
  }, INVESTIGATION_REFRESH_DEBOUNCE_MS);
}

const sse = useSSE(
  "/api/v1/events",
  {
    alert_created: handleAlertCreatedSSE,
    alert_updated: handleAlertUpdatedSSE,
    alert_deleted: handleAlertDeletedSSE,
    investigation_created: scheduleInvestigationRefresh,
    investigation_started: scheduleInvestigationRefresh,
    investigation_status_changed: scheduleInvestigationRefresh,
    investigation_complete: scheduleInvestigationRefresh,
    investigation_patch: scheduleInvestigationRefresh,
    investigation_update: scheduleInvestigationRefresh,
  },
  {
    onReconnect: () => {
      lastLoadedQuery = "";
      load(true);
    },
  },
);

function handleAlertCreated(created: AlertRecord) {
  if (alerts.value.some((a) => a.alert_number === created.alert_number)) return;
  if (passesListFilters(created)) {
    alerts.value = sortAlerts([created, ...alerts.value], sortBy.value);
    markSseAppendedHighlight(created.fingerprint);
  }
}

function clearAllFilters() {
  filterSync.clearFilters(() => {
    status.value = "all";
    timeRange.value = "all";
    customStart.value = "";
    customEnd.value = "";
    searchInput.value = "";
    sortBy.value = DEFAULT_ALERT_SORT;
  });
}

const statusLabel = computed(() => {
  switch (status.value) {
    case "open":
      return "Open";
    case "closed":
      return "Resolved";
    case "unacknowledged":
      return "Unacknowledged";
    case "acknowledged":
      return "Acknowledged";
    default:
      return "All";
  }
});

function resolveTimeRange() {
  if (timeRange.value === "all") {
    return { startDate: "", endDate: "" };
  }
  if (timeRange.value === "custom") {
    return { startDate: customStart.value, endDate: customEnd.value };
  }

  const now = new Date();
  const start = new Date(now);
  switch (timeRange.value) {
    case "1h":
      start.setHours(start.getHours() - 1);
      break;
    case "24h":
      start.setDate(start.getDate() - 1);
      break;
    case "7d":
      start.setDate(start.getDate() - 7);
      break;
    case "30d":
      start.setDate(start.getDate() - 30);
      break;
    case "1y":
      start.setFullYear(start.getFullYear() - 1);
      break;
  }
  return { startDate: start.toISOString(), endDate: now.toISOString() };
}

let lastLoadedQuery = "";

async function load(reset = true) {
  if (reset) {
    const currentQuery = JSON.stringify(buildFilterParams());
    if (currentQuery === lastLoadedQuery && alerts.value.length > 0) return;
    lastLoadedQuery = currentQuery;
    loading.value = true;
  } else {
    loadMoreLoading.value = true;
  }
  error.value = "";
  try {
    const range = resolveTimeRange();
    const q = searchInput.value.trim();
    const data = await api.getAlerts({
      limit: PAGE_SIZE,
      skip: reset ? 0 : alerts.value.length,
      status: status.value || undefined,
      start_date: range.startDate ? new Date(range.startDate).toISOString() : undefined,
      end_date: range.endDate ? new Date(range.endDate).toISOString() : undefined,
      search: q || undefined,
      sort: sortBy.value || undefined,
    });
    const batch = Array.isArray(data) ? data : [];

    if (reset) {
      const batchNumbers = new Set(batch.map((a) => a.alert_number));
      const preserved = alerts.value.filter(
        (a) => !batchNumbers.has(a.alert_number) && passesListFilters(a),
      );
      alerts.value = [...batch, ...preserved];
    } else {
      const seen = new Set(alerts.value.map((a) => a.alert_number));
      for (const a of batch) {
        if (!seen.has(a.alert_number)) {
          alerts.value.push(a);
          seen.add(a.alert_number);
        }
      }
    }
    hasMore.value = batch.length >= PAGE_SIZE;
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to load alerts");
    push(getErrorMessage(err, "Failed to load alerts"), "error");
  } finally {
    loading.value = false;
    loadMoreLoading.value = false;
  }
}

function applyCustomRange() {
  if (!customStart.value || !customEnd.value) {
    push("Start and end date/time are required", "error");
    return;
  }
  if (new Date(customStart.value) > new Date(customEnd.value)) {
    push("Start time must be before end time", "error");
    return;
  }
  timeRange.value = "custom";
  showCustomRange.value = false;
  filterSync.syncFiltersToUrl();
  load(true);
}

function loadMore() {
  if (!hasMore.value || loadMoreLoading.value || loading.value) return;
  load(false);
}

const { scheduleSearchReload } = useSearchDebounce(() => {
  filterSync.syncFiltersToUrl();
  load(true);
});

const { showFilters } = usePageHeaderActions({
  title: "Alerts",
  titleIcon: Bell,
  searchInput,
  searchPlaceholder: "Search alerts...",
  onSearchInput: scheduleSearchReload,
  showFilters: true,
  hasNonDefaultFilters,
  showAdd: canWrite,
  onAdd: () => {
    showCreateDialog.value = true;
  },
  addLabel: "Create alert",
});

let mounted = false;
const scrollSentinelRef = ref<HTMLElement | null>(null);
useScrollRestore({ skipFirst: true });
let isDeactivated = false;

const { setup: setupScrollObserver, teardown: teardownScrollObserver } = useInfiniteScroll(
  scrollSentinelRef,
  () => hasMore.value,
  () => loadMore(),
  { rootMargin: "200px" },
);

onMounted(() => {
  mounted = true;
});

// Unconditional KeepAlive fires onActivated after the first onMounted too, so
// the initial load happens here only — otherwise the first mount issues two
// identical list fetches that race.
onActivated(() => {
  isDeactivated = false;
  navigatingAlertNumber.value = null;
  lastLoadedQuery = "";
  sse.reconnect();
  nextTick(() => {
    setupScrollObserver();
  });
  load(true);
});

onDeactivated(() => {
  isDeactivated = true;
  lastLoadedQuery = "";
  sse.close();
  teardownScrollObserver();
});

watch(
  () => route.query,
  () => {
    if (!mounted || isDeactivated || route.path !== "/alerts") return;
    filterSync.applyFromUrl();
    load(true);
  },
);

watch(status, () => {
  if (filterSync.syncingFromRoute.value) return;
  filterSync.syncFiltersToUrl();
  load(true);
});

watch(timeRange, (val) => {
  if (filterSync.syncingFromRoute.value) return;
  filterSync.syncFiltersToUrl();
  if (val !== "custom") load(true);
});

watch(sortBy, () => {
  if (filterSync.syncingFromRoute.value) return;
  filterSync.syncFiltersToUrl();
  load(true);
});

onUnmounted(() => {
  for (const t of sseHighlightClearTimers.values()) clearTimeout(t);
  sseHighlightClearTimers.clear();
  teardownScrollObserver();
  if (investigationRefreshTimer != null) {
    clearTimeout(investigationRefreshTimer);
    investigationRefreshTimer = null;
  }
  lastLoadedQuery = "";
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <Card v-if="showFilters" class="!p-4">
      <div class="flex flex-wrap items-end gap-3">
        <div class="min-w-0 w-full sm:w-auto sm:min-w-[12rem]">
          <FormLabel for="alerts-status-filter">Status</FormLabel>
          <Select
            id="alerts-status-filter"
            v-model="status"
            class="w-full"
            aria-label="Filter by status"
          >
            <option value="all">All alerts</option>
            <option value="open">Open</option>
            <option value="closed">Resolved</option>
            <option value="unacknowledged">Unacknowledged</option>
            <option value="acknowledged">Acknowledged</option>
          </Select>
        </div>
        <div class="min-w-0 w-full sm:w-auto sm:min-w-[14rem]">
          <FormLabel for="alerts-time-range-filter">Time range</FormLabel>
          <Select
            id="alerts-time-range-filter"
            v-model="timeRange"
            class="w-full"
            aria-label="Filter by time range"
            @update:model-value="(value: string) => value === 'custom' && (showCustomRange = true)"
          >
            <option value="all">All time</option>
            <option value="1h">Last 1 hour</option>
            <option value="24h">Last 24 hours</option>
            <option value="7d">Last 7 days</option>
            <option value="30d">Last 30 days</option>
            <option value="1y">Last 1 year</option>
            <option value="custom">Custom range</option>
          </Select>
        </div>
        <div class="min-w-0 w-full sm:w-auto sm:min-w-[12rem]">
          <FormLabel for="alerts-sort-filter">Sort</FormLabel>
          <SortSelect
            id="alerts-sort-filter"
            v-model="sortBy"
            :options="ALERT_SORT_OPTIONS"
            :default-sort="DEFAULT_ALERT_SORT"
            class="w-full"
          />
        </div>
        <Button
          v-if="hasNonDefaultFilters"
          variant="outline"
          size="sm"
          type="button"
          @click="clearAllFilters"
        >
          <X class="h-3.5 w-3.5" aria-hidden="true" />
          Clear filters
        </Button>
      </div>
    </Card>
    <p class="text-xs text-[var(--text-muted)]">
      Showing: {{ statusLabel }}
      <template v-if="timeRange !== 'all'">
        · {{ timeRange === "custom" ? "Custom range" : timeRange }}
      </template>
    </p>

    <ErrorBanner :message="error" />
    <LoadingSpinner v-if="loading && alerts.length === 0" centered />

    <SkeletonRows v-if="loading && alerts.length === 0" :count="4" />

    <div v-if="!(loading && alerts.length === 0)" class="space-y-3">
      <InteractiveCard
        v-for="alert in alerts"
        :key="alert.alert_number"
        :loading="navigatingAlertNumber === String(alert.alert_number)"
        :class="[
          'group/alert flex !p-0 !rounded',
          isSseAppendedHighlight(alert.fingerprint) ? 'alert-card-sse-appended' : '',
        ]"
        @navigate="alert.alert_number != null && navigateToAlert(alert.alert_number)"
      >
        <div
          :class="severityRailClass(alertSeverityLabel(alert.labels))"
          class="flex w-6 shrink-0 items-center justify-center rounded-l border-r text-[0.55rem] font-semibold uppercase tracking-[0.14em]"
          aria-hidden="true"
        >
          <span class="-rotate-90 whitespace-nowrap">
            {{ alertSeverityLabel(alert.labels) || "severity" }}
          </span>
        </div>
        <div class="min-w-0 flex-1 p-3 sm:p-4">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="flex min-w-0 items-center gap-2">
                <span
                  v-if="alert.alert_number != null && alert.alert_number > 0"
                  class="shrink-0 font-mono text-xs text-[var(--text-muted)]"
                >
                  #{{ alert.alert_number }}
                </span>
                <span class="min-w-0 truncate font-medium text-[var(--text-primary)]">
                  {{ alert.labels?.alertname ?? alert.fingerprint }}
                </span>
              </div>
              <div
                class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--text-muted)]"
              >
                <span class="flex items-center gap-1">
                  <Clock class="h-3 w-3" />
                  {{ formatTimeFull(alert.starts_at) }}
                </span>
                <template v-if="investigationActor(alert)">
                  <span aria-hidden="true" class="text-[var(--text-muted)]/60">·</span>
                  <span
                    class="flex items-center gap-1"
                    :title="`Investigating: ${investigationActor(alert)?.name}`"
                  >
                    <Bot
                      v-if="investigationActor(alert)?.isAgent"
                      class="h-3 w-3 text-[var(--text-muted)]"
                      aria-hidden="true"
                    />
                    <User v-else class="h-3 w-3 text-[var(--text-muted)]" aria-hidden="true" />
                    <span class="font-medium text-[var(--text-primary)]">
                      {{ investigationActor(alert)?.name }}
                    </span>
                  </span>
                </template>
              </div>
              <p
                v-if="alert.annotations?.summary || alert.annotations?.description"
                class="mt-1.5 line-clamp-2 text-sm text-[var(--text-secondary)]"
              >
                {{ alert.annotations.summary || alert.annotations.description }}
              </p>
              <div
                v-if="allLabelKeys(alert).length > 0"
                class="mt-2 flex flex-wrap items-center gap-1.5"
              >
                <span
                  v-for="key in limitedLabelKeys(alert)"
                  :key="key"
                  class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-1.5 py-0.5 text-xs"
                >
                  <span class="text-[var(--text-muted)]">{{ key }}:</span> {{ alert.labels[key] }}
                </span>
                <span v-if="hiddenLabelCount(alert) > 0" class="text-xs text-[var(--text-muted)]">
                  +{{ hiddenLabelCount(alert) }} more
                </span>
              </div>
            </div>
            <div class="flex shrink-0 flex-col items-end gap-2 self-stretch">
              <div class="flex items-center gap-1.5">
                <AlertStatusBadge
                  :status="alert.status"
                  :acknowledged="alert.acknowledged"
                  class="uppercase"
                />
              </div>
              <div class="mt-auto flex items-center gap-1.5">
                <Button
                  v-if="alert.status === 'firing' && !alert.acknowledged && canWrite"
                  variant="outline"
                  size="sm"
                  :disabled="!!(alert.alert_number != null && ackLoadingMap[alert.alert_number])"
                  @click.stop="acknowledgeAlert(alert)"
                >
                  <ShieldCheck class="h-3.5 w-3.5" aria-hidden="true" />
                  {{
                    alert.alert_number != null && ackLoadingMap[alert.alert_number] ? "..." : "Ack"
                  }}
                </Button>
                <AlertActionsMenu
                  v-if="canWrite || canDelete"
                  :workflow-status="alert.status === 'resolved' ? 'resolved' : 'open'"
                  :status-busy="
                    !!(alert.alert_number != null && resolveLoadingMap[alert.alert_number])
                  "
                  :can-write="canWrite"
                  :can-delete="canDelete"
                  :can-create-incident="false"
                  :show-ack-button="alert.status === 'firing' && !alert.acknowledged"
                  icon="horizontal"
                  @resolve="resolveAlert(alert)"
                  @delete="confirmDelete(alert)"
                />
              </div>
            </div>
          </div>
        </div>
      </InteractiveCard>
      <EmptyState v-if="alerts.length === 0 && !loading" message="No alerts found.">
        <template #icon>
          <CircleAlert class="mb-2 h-6 w-6 opacity-40" />
        </template>
        <template v-if="status !== 'all' || timeRange !== 'all' || searchInput" #footer>
          <p class="mt-1 text-xs text-[var(--text-muted)]">
            Try adjusting your filters or
            <button
              type="button"
              class="text-[var(--text-link)] hover:underline"
              @click="clearAllFilters"
            >
              clear all filters</button
            >.
          </p>
        </template>
      </EmptyState>
      <div
        v-if="hasMore && alerts.length > 0"
        ref="scrollSentinelRef"
        class="flex justify-center py-4"
      >
        <LoadingSpinner v-if="loadMoreLoading" label="" />
      </div>
    </div>

    <Modal
      :open="showCustomRange"
      title="Custom range"
      max-width="lg"
      :show-footer="false"
      @update:open="showCustomRange = $event"
    >
      <div class="grid gap-3 md:grid-cols-2">
        <div>
          <FormLabel for="custom-range-start">Start</FormLabel>
          <DateTimePicker
            id="custom-range-start"
            v-model="customStart"
            placeholder="Start date & time"
          />
        </div>
        <div>
          <FormLabel for="custom-range-end">End</FormLabel>
          <DateTimePicker id="custom-range-end" v-model="customEnd" placeholder="End date & time" />
        </div>
      </div>
      <template #footer>
        <Button variant="outline" @click="showCustomRange = false">Cancel</Button>
        <Button variant="primary" @click="applyCustomRange">Apply</Button>
      </template>
    </Modal>

    <CreateAlertDialog
      v-if="canWrite"
      :open="showCreateDialog"
      @update:open="(v: boolean) => (showCreateDialog = v)"
      @created="handleAlertCreated"
    />

    <ConfirmDialog
      :open="showDeleteConfirm"
      title="Delete alert"
      message="Are you sure you want to delete this alert? This cannot be undone."
      confirm-label="Delete"
      :destructive="true"
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />
  </section>
</template>
