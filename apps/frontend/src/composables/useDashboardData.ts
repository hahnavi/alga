import { onBeforeUnmount, onDeactivated, ref, type Ref } from "vue";
import {
  api,
  type DashboardStats,
  type DailySummaryResponse,
  type IncidentMetrics,
  type OnCallCurrent,
  type ServiceRecord,
} from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useSSE } from "@/composables/useSSE";

export type DateRange = "24h" | "7d" | "30d" | "90d";

/**
 * Owns the dashboard's five data loaders (stats, incident metrics, on-call,
 * services, daily summary) plus the refresh-debounced SSE wiring. Kept as a
 * composable so the dashboard view stops owning data fetching.
 */
export function useDashboardData(selectedRange: Ref<DateRange>) {
  const stats = ref<DashboardStats | null>(null);
  const metrics = ref<IncidentMetrics | null>(null);
  const onCallEntries = ref<OnCallCurrent[]>([]);
  const services = ref<ServiceRecord[]>([]);
  const loading = ref(true);
  const loadError = ref("");
  const summary = ref<DailySummaryResponse | null>(null);
  const summaryLoading = ref(false);
  const summaryError = ref("");
  const summaryExpanded = ref(false);

  let refreshDebounce: number | undefined;
  const REFRESH_DEBOUNCE_MS = 1500;

  function dateRangeToISO(range: DateRange): { start: string; end: string } {
    const now = new Date();
    const end = now.toISOString().split("T")[0];
    const start = new Date(now);
    if (range === "24h") start.setDate(start.getDate() - 1);
    else if (range === "7d") start.setDate(start.getDate() - 7);
    else if (range === "30d") start.setDate(start.getDate() - 30);
    else start.setDate(start.getDate() - 90);
    return { start: start.toISOString().split("T")[0], end };
  }

  function formatMinutes(m: number): string {
    if (!m && m !== 0) return "—";
    if (m < 60) return m.toFixed(1) + " min";
    const hours = Math.floor(m / 60);
    const mins = Math.round(m % 60);
    return `${hours}h ${mins}m`;
  }

  async function loadStats() {
    try {
      stats.value = await api.getDashboardStats();
    } catch (e: unknown) {
      loadError.value = getErrorMessage(e, "Failed to load dashboard stats");
    } finally {
      loading.value = false;
    }
  }

  async function loadMetrics() {
    try {
      const { start, end } = dateRangeToISO(selectedRange.value);
      metrics.value = await api.getIncidentMetrics(start, end);
    } catch {
      metrics.value = null;
    }
  }

  async function loadOnCall() {
    try {
      onCallEntries.value = await api.getWhoIsOnCall();
    } catch {
      onCallEntries.value = [];
    }
  }

  async function loadServices() {
    try {
      const result = await api.getServices({ limit: 20 });
      services.value = result.items ?? [];
    } catch {
      services.value = [];
    }
  }

  async function loadSummary() {
    try {
      summary.value = await api.getDailySummary();
    } catch {
      summary.value = null;
    }
  }

  async function generateSummary() {
    summaryLoading.value = true;
    summaryError.value = "";
    try {
      summary.value = await api.generateDailySummary();
    } catch (e: unknown) {
      summaryError.value = getErrorMessage(e, "Failed to generate summary");
    } finally {
      summaryLoading.value = false;
    }
  }

  function setRange(range: DateRange) {
    selectedRange.value = range;
    loadMetrics();
  }

  function scheduleRefresh() {
    if (refreshDebounce != null) clearTimeout(refreshDebounce);
    refreshDebounce = setTimeout(() => {
      refreshDebounce = undefined;
      loadStats();
      loadMetrics();
    }, REFRESH_DEBOUNCE_MS);
  }

  useSSE("/api/v1/events", {
    alert_created: () => scheduleRefresh(),
    alert_updated: () => scheduleRefresh(),
    alert_deleted: () => scheduleRefresh(),
    investigation_update: () => scheduleRefresh(),
    investigation_started: () => scheduleRefresh(),
    investigation_complete: () => scheduleRefresh(),
    investigation_deleted: () => scheduleRefresh(),
    investigation_patch: () => scheduleRefresh(),
  });

  function clearRefreshDebounce() {
    if (refreshDebounce != null) clearTimeout(refreshDebounce);
  }

  onDeactivated(clearRefreshDebounce);
  onBeforeUnmount(clearRefreshDebounce);

  return {
    stats,
    metrics,
    onCallEntries,
    services,
    loading,
    loadError,
    summary,
    summaryLoading,
    summaryError,
    summaryExpanded,
    loadStats,
    loadMetrics,
    loadOnCall,
    loadServices,
    loadSummary,
    generateSummary,
    setRange,
    formatMinutes,
  };
}
