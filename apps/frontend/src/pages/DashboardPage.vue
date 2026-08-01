<script setup lang="ts">
import { onMounted, ref, computed, h, watch } from "vue";
import { useRouter } from "vue-router";
import {
  Flame,
  AlertTriangle,
  Search,
  Sparkles,
  Loader2,
  Clock,
  Bot,
  ChevronRight,
  ChevronDown,
  ShieldAlert,
  ShieldCheck,
  AlertOctagon,
  Timer,
  BarChart3,
  Activity,
  Server,
  TrendingUp,
  Radio,
} from "@lucide/vue";
import { Doughnut, Line, Bar } from "vue-chartjs";
import type { ChartOptions } from "chart.js";
import { useChartOptions } from "@/composables/useChartOptions";
import { useGlobalSearch } from "@/composables/useGlobalSearch";
import { useDashboardData, type DateRange } from "@/composables/useDashboardData";
import { useTheme } from "@/lib/theme";
import {
  investigationStatusBadgeClass,
  investigationStatusLabel,
  incidentPriorityBadgeClass,
  incidentStatusBadgeClass,
  incidentStatusLabel,
  serviceStatusBadgeClass,
  serviceStatusLabel,
} from "@/lib/alertLabels";
import { formatTimeAgo, formatDurationFromMs } from "@/lib/time";
import { investigationDisplayId } from "@/lib/investigation";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import MarkdownRenderer from "@/components/ui/MarkdownRenderer.vue";
import NotificationBell from "@/components/NotificationBell.vue";
import { usePageHeader } from "@/composables/usePageHeader";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";

defineOptions({ name: "DashboardPage" });

const { lineChartOptions, doughnutChartOptions, colors: chartColors } = useChartOptions();
const { isDark } = useTheme();
const router = useRouter();

const { openGlobalSearch } = useGlobalSearch();

const selectedRange = ref<DateRange>("30d");
const {
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
} = useDashboardData(selectedRange);

watch(summary, (s) => {
  if (s?.available && s.summary) summaryExpanded.value = true;
});

usePageHeader(() => ({
  title: "Dashboard",
  options: {
    actions: [
      h(
        "div",
        {
          class: "flex items-center gap-1",
        },
        (["24h", "7d", "30d", "90d"] as DateRange[]).map((range) =>
          h(
            "button",
            {
              type: "button",
              key: range,
              class: `cursor-pointer rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                selectedRange.value === range
                  ? "bg-[var(--focus-ring)] text-white"
                  : "bg-[var(--bg-secondary)] text-[var(--text-muted)] hover:text-[var(--text-secondary)]"
              }`,
              onClick: () => {
                setRange(range);
              },
            },
            [range],
          ),
        ),
      ),
      h(NotificationBell),
      h(
        "button",
        {
          type: "button",
          class: HEADER_ICON_BTN_CLASS,
          "aria-label": "Search",
          title: "Search (Ctrl+K)",
          onClick: () => {
            openGlobalSearch();
          },
        },
        [h(Search, { class: "h-4 w-4", "aria-hidden": "true" })],
      ),
    ],
    titleIcon: h(BarChart3, {
      class: "h-5 w-5 shrink-0 text-[var(--text-muted)]",
      "aria-hidden": "true",
    }),
  },
}));

onMounted(() => {
  void Promise.all([loadStats(), loadMetrics(), loadOnCall(), loadServices(), loadSummary()]);
});

function navigateToAlerts(filter: Record<string, string>) {
  router.push({ path: "/alerts", query: filter });
}

function navigateToIncidents(filter: Record<string, string>) {
  router.push({ path: "/incidents", query: filter });
}

function handleCardKeydown(e: KeyboardEvent, handler: () => void) {
  if (e.key === "Enter" || e.key === " ") {
    e.preventDefault();
    handler();
  }
}

const slaColor = computed(() => {
  if (!metrics.value) return "var(--text-primary)";
  const pct = metrics.value.sla_compliance.resolve_sla_compliance_pct;
  if (pct > 95) return "var(--text-badge-resolved)";
  if (pct > 80) return "var(--text-badge-warning)";
  return "var(--text-badge-firing)";
});

const incidentTrendChartData = computed(() => {
  if (!metrics.value || !metrics.value.trend || metrics.value.trend.length === 0) return null;
  const trend = metrics.value.trend;
  const c = chartColors.value;
  return {
    labels: trend.map((t) => {
      const parts = t.date.split("-");
      return `${parts[1]}/${parts[2]}`;
    }),
    datasets: [
      {
        label: "Created",
        data: trend.map((t) => t.created),
        borderColor: c["--chart-blue"],
        backgroundColor: c["--chart-blue-fill"],
        fill: true,
        tension: 0.3,
        pointRadius: 3,
        pointHoverRadius: 5,
      },
      {
        label: "Resolved",
        data: trend.map((t) => t.resolved),
        borderColor: c["--chart-green"],
        backgroundColor: c["--chart-green-fill"],
        fill: true,
        tension: 0.3,
        pointRadius: 3,
        pointHoverRadius: 5,
      },
    ],
  };
});

const severityChartData = computed(() => {
  if (!stats.value || stats.value.alerts_by_severity.length === 0) return null;
  const buckets = stats.value.alerts_by_severity;
  const c = chartColors.value;
  const colorMap: Record<string, string> = {
    critical: c["--chart-red"],
    high: c["--chart-red"],
    error: c["--chart-red"],
    fatal: c["--chart-red"],
    emergency: c["--chart-red"],
    warning: c["--chart-amber"],
    warn: c["--chart-amber"],
    medium: c["--chart-amber"],
    info: c["--chart-green"],
    low: c["--chart-green"],
    unknown: c["--chart-gray"],
  };
  return {
    labels: buckets.map((b) => b.severity),
    datasets: [
      {
        data: buckets.map((b) => b.count),
        backgroundColor: buckets.map((b) => colorMap[b.severity] ?? c["--chart-gray"]),
        borderWidth: 0,
        hoverOffset: 6,
      },
    ],
  };
});

const incidentPriorityChartData = computed(() => {
  if (
    !stats.value ||
    !stats.value.incidents.by_priority ||
    Object.keys(stats.value.incidents.by_priority).length === 0
  )
    return null;
  const buckets = stats.value.incidents.by_priority;
  const c = chartColors.value;
  const colorMap: Record<string, string> = {
    P1: c["--chart-red"],
    P2: c["--chart-orange"],
    P3: c["--chart-amber"],
    P4: c["--chart-blue"],
    P5: c["--chart-gray"],
  };
  return {
    labels: Object.keys(buckets),
    datasets: [
      {
        data: Object.values(buckets),
        backgroundColor: Object.keys(buckets).map((k) => colorMap[k] ?? c["--chart-gray"]),
        borderWidth: 0,
        hoverOffset: 6,
      },
    ],
  };
});

const chartCardCount = computed(() => (stats.value?.incidents.by_priority ? 5 : 4));

const chartGridClass = computed(() =>
  chartCardCount.value % 2 === 1
    ? "grid grid-cols-1 gap-4 md:grid-cols-2 md:[&>:last-child]:col-span-2 xl:grid-cols-6 xl:[&>*]:col-span-2 xl:[&>:last-child]:col-span-3 xl:[&>:nth-last-child(2)]:col-span-3"
    : "grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4",
);

const serviceNameLookup = computed(() => {
  const map = new Map<string, string>();
  for (const s of services.value) {
    map.set(s.id, s.display_name || s.name);
  }
  return map;
});

const topServicesChartData = computed(() => {
  if (!metrics.value || Object.keys(metrics.value.by_service).length === 0) return null;
  const rows = Object.entries(metrics.value.by_service)
    .map(([service, data]) => ({ service, count: data.count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 8);
  const c = chartColors.value;
  return {
    labels: rows.map((r) => serviceNameLookup.value.get(r.service) ?? r.service.slice(0, 8)),
    datasets: [
      {
        label: "Incidents",
        data: rows.map((r) => r.count),
        backgroundColor: c["--chart-blue"],
        borderRadius: 4,
        barThickness: 20,
      },
    ],
  };
});

const barChartOptions = computed<ChartOptions<"bar">>(() => {
  const vars = {
    "--text-secondary": "",
    "--text-muted": "",
    "--chart-grid-dark": "",
    "--chart-grid-light": "",
  };
  const style = getComputedStyle(document.documentElement);
  for (const k of Object.keys(vars)) {
    vars[k as keyof typeof vars] = style.getPropertyValue(k).trim();
  }
  void isDark.value;
  const gridColor = isDark.value
    ? chartColors.value["--chart-grid-dark"]
    : chartColors.value["--chart-grid-light"];
  return {
    indexAxis: "y",
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: {
        beginAtZero: true,
        ticks: { color: vars["--text-muted"], font: { size: 11 }, stepSize: 1 },
        grid: { color: gridColor },
      },
      y: {
        ticks: { color: vars["--text-muted"], font: { size: 11 } },
        grid: { display: false },
      },
    },
    plugins: {
      legend: { display: false },
      tooltip: {
        backgroundColor: "rgba(0,0,0,0.8)",
        titleColor: "#fff",
        bodyColor: "#fff",
        padding: 10,
        cornerRadius: 6,
      },
    },
  };
});

const mttaMttrChartData = computed(() => {
  if (!metrics.value || !metrics.value.trend || metrics.value.trend.length === 0) return null;
  const trend = metrics.value.trend;
  const c = chartColors.value;
  return {
    labels: trend.map((t) => {
      const parts = t.date.split("-");
      return `${parts[1]}/${parts[2]}`;
    }),
    datasets: [
      {
        label: "MTTA",
        data: trend.map((t) => t.mtta_minutes),
        borderColor: c["--chart-amber"],
        backgroundColor: c["--chart-amber-fill"],
        fill: true,
        tension: 0.3,
        pointRadius: 3,
        pointHoverRadius: 5,
      },
      {
        label: "MTTR",
        data: trend.map((t) => t.mttr_minutes),
        borderColor: c["--chart-red"],
        backgroundColor: c["--chart-red-fill"],
        fill: true,
        tension: 0.3,
        pointRadius: 3,
        pointHoverRadius: 5,
      },
    ],
  };
});

const investigationBreakdown = computed(() => {
  if (!stats.value) return [];
  const inv = stats.value.investigations;
  const total = inv.total || 1;
  return [
    { label: "Complete", value: inv.complete, cls: "bg-[var(--text-badge-resolved)]" },
    { label: "Investigating", value: inv.investigating, cls: "bg-[var(--text-badge-warning)]" },
    { label: "Pending", value: inv.pending, cls: "bg-[var(--text-badge-pending)]" },
    { label: "Failed", value: inv.failed, cls: "bg-[var(--text-badge-firing)]" },
    { label: "Cancelled", value: inv.cancelled, cls: "bg-[var(--text-muted)]" },
    { label: "Timed out", value: inv.timed_out, cls: "bg-[var(--text-muted)]" },
  ].map((item) => ({
    ...item,
    pct: ((item.value / total) * 100).toFixed(1),
  }));
});

const nonOperationalServices = computed(() =>
  services.value.filter((s) => s.status !== "operational"),
);

const operationalServices = computed(() =>
  services.value.filter((s) => s.status === "operational"),
);

const recentActivity = computed(() => {
  if (!stats.value) return [];
  type ActivityItem = {
    key: string;
    type: "investigation" | "incident";
    title: string;
    subtitle: string;
    status: string;
    statusBadgeClass: string;
    statusLabel: string;
    severity?: string;
    created_at: string;
    route: string;
  };
  const items: ActivityItem[] = [];

  for (const inv of stats.value.recent_investigations.slice(0, 8)) {
    items.push({
      key: inv.investigation_id,
      type: "investigation",
      title: inv.alert_name || inv.correlation_key,
      subtitle: investigationDisplayId(inv),
      status: inv.status,
      statusBadgeClass: investigationStatusBadgeClass(inv.status),
      statusLabel: investigationStatusLabel(inv.status),
      created_at: inv.created_at,
      route: "",
    });
  }

  for (const inc of (stats.value.active_incidents ?? []).slice(0, 5)) {
    items.push({
      key: String(inc.incident_number),
      type: "incident",
      title: `#${inc.incident_number} ${inc.title}`,
      subtitle: inc.service_name ?? "",
      status: inc.status,
      statusBadgeClass: incidentStatusBadgeClass(inc.status),
      statusLabel: incidentStatusLabel(inc.status),
      severity: inc.severity,
      created_at: inc.created_at,
      route: `/incidents/${inc.incident_number}`,
    });
  }

  items.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
  return items.slice(0, 15);
});
</script>

<template>
  <div class="mx-auto max-w-7xl space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6 lg:px-8">
    <template v-if="loading">
      <div
        class="grid grid-cols-2 gap-3 [&>:last-child]:col-span-2 sm:grid-cols-6 sm:[&>*]:col-span-2 sm:[&>:last-child]:col-span-3 sm:[&>:nth-last-child(2)]:col-span-3 lg:grid-cols-5 lg:[&>*]:col-span-1 lg:[&>:last-child]:col-span-1 lg:[&>:nth-last-child(2)]:col-span-1"
      >
        <div
          v-for="i in 5"
          :key="i"
          class="min-h-20 animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)] p-4"
        >
          <div class="flex items-center gap-3">
            <div class="h-9 w-9 shrink-0 rounded-lg bg-[var(--skeleton-bg)]"></div>
            <div class="space-y-2">
              <div class="h-5 w-14 rounded bg-[var(--skeleton-bg)]"></div>
              <div class="h-3 w-16 rounded bg-[var(--skeleton-bg)]"></div>
            </div>
          </div>
        </div>
      </div>
      <div class="grid gap-4 lg:grid-cols-2">
        <div
          v-for="i in 4"
          :key="i"
          class="h-56 animate-pulse rounded border border-[var(--border-primary)] bg-[var(--bg-card)]"
        ></div>
      </div>
    </template>

    <ErrorBanner v-else-if="loadError" :message="loadError" />

    <template v-else-if="stats">
      <div
        class="rise grid grid-cols-2 gap-3 [&>:last-child]:col-span-2 sm:grid-cols-6 sm:[&>*]:col-span-2 sm:[&>:last-child]:col-span-3 sm:[&>:nth-last-child(2)]:col-span-3 lg:grid-cols-5 lg:[&>*]:col-span-1 lg:[&>:last-child]:col-span-1 lg:[&>:nth-last-child(2)]:col-span-1"
      >
        <Card
          class="min-h-20 cursor-pointer transition-all duration-150 hover:border-[var(--border-secondary)] hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
          role="button"
          tabindex="0"
          @click="navigateToIncidents({})"
          @keydown="handleCardKeydown($event, () => navigateToIncidents({}))"
        >
          <div class="flex h-full min-w-0 items-center gap-2.5">
            <div
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--chart-amber)]/15"
            >
              <Timer class="h-4 w-4 text-[var(--chart-amber)]" />
            </div>
            <div class="min-w-0">
              <div
                class="truncate text-lg font-bold tracking-tight tabular-nums text-[var(--text-primary)]"
              >
                {{ metrics ? formatMinutes(metrics.mtta_minutes) : "\u2014" }}
              </div>
              <div class="eyebrow text-[10px]">MTTA</div>
            </div>
          </div>
        </Card>

        <Card
          class="min-h-20 cursor-pointer transition-all duration-150 hover:border-[var(--border-secondary)] hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
          role="button"
          tabindex="0"
          @click="navigateToIncidents({})"
          @keydown="handleCardKeydown($event, () => navigateToIncidents({}))"
        >
          <div class="flex h-full min-w-0 items-center gap-2.5">
            <div
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--chart-red)]/15"
            >
              <Clock class="h-4 w-4 text-[var(--chart-red)]" />
            </div>
            <div class="min-w-0">
              <div
                class="truncate text-lg font-bold tracking-tight tabular-nums text-[var(--text-primary)]"
              >
                {{ metrics ? formatMinutes(metrics.mttr_minutes) : "\u2014" }}
              </div>
              <div class="eyebrow text-[10px]">MTTR</div>
            </div>
          </div>
        </Card>

        <Card
          class="min-h-20 cursor-pointer transition-all duration-150 hover:border-[var(--border-secondary)] hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
          role="button"
          tabindex="0"
          @click="navigateToAlerts({ status: 'open' })"
          @keydown="handleCardKeydown($event, () => navigateToAlerts({ status: 'open' }))"
        >
          <div class="flex h-full min-w-0 items-center gap-2.5">
            <div
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--text-badge-firing)]/15"
            >
              <Flame class="h-4 w-4 text-[var(--text-badge-firing)]" />
            </div>
            <div class="min-w-0">
              <div
                class="truncate text-lg font-bold tracking-tight tabular-nums text-[var(--text-badge-firing)]"
              >
                {{ stats.alerts.firing }}
              </div>
              <div class="eyebrow text-[10px]">Firing</div>
            </div>
          </div>
        </Card>

        <Card
          class="min-h-20 cursor-pointer transition-all duration-150 hover:border-[var(--border-secondary)] hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
          role="button"
          tabindex="0"
          @click="navigateToIncidents({ status: 'active' })"
          @keydown="handleCardKeydown($event, () => navigateToIncidents({ status: 'active' }))"
        >
          <div class="flex h-full min-w-0 items-center gap-2.5">
            <div
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--text-badge-warning)]/15"
            >
              <AlertOctagon class="h-4 w-4 text-[var(--text-badge-warning)]" />
            </div>
            <div class="min-w-0">
              <div
                class="truncate text-lg font-bold tracking-tight tabular-nums text-[var(--text-primary)]"
              >
                {{ stats.incidents.active }}
              </div>
              <div class="eyebrow truncate text-[10px]">Active Incidents</div>
            </div>
          </div>
        </Card>

        <Card
          class="min-h-20 cursor-pointer transition-all duration-150 hover:border-[var(--border-secondary)] hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
          role="button"
          tabindex="0"
          @click="navigateToIncidents({})"
          @keydown="handleCardKeydown($event, () => navigateToIncidents({}))"
        >
          <div class="flex h-full min-w-0 items-center gap-2.5">
            <div
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
              :style="{ backgroundColor: slaColor + '18' }"
            >
              <ShieldCheck class="h-4 w-4" :style="{ color: slaColor }" />
            </div>
            <div class="min-w-0">
              <div
                class="truncate text-lg font-bold tracking-tight tabular-nums"
                :style="{ color: slaColor }"
              >
                {{
                  metrics
                    ? metrics.sla_compliance.resolve_sla_compliance_pct.toFixed(1) + "%"
                    : "\u2014"
                }}
              </div>
              <div class="eyebrow text-[10px]">SLA</div>
            </div>
          </div>
        </Card>
      </div>

      <Card class="rise [animation-delay:60ms]">
        <div class="flex w-full items-center gap-2">
          <h2 class="eyebrow min-w-0 flex-1">
            <button
              type="button"
              class="flex w-full cursor-pointer items-center justify-between"
              :aria-expanded="summaryExpanded"
              @click="summaryExpanded = !summaryExpanded"
            >
              <span class="flex items-center gap-2">
                <Sparkles class="h-4 w-4 text-[var(--focus-ring)]" />
                Daily Write-Up
              </span>
              <span class="flex items-center gap-2">
                <span
                  v-if="summary && summary.available"
                  class="hidden items-center gap-1 text-[10px] font-normal normal-case tracking-normal text-[var(--text-muted)] sm:flex"
                >
                  <Clock class="h-3 w-3" />
                  {{ formatTimeAgo(summary.generated_at) }}
                </span>
                <ChevronDown
                  class="h-4 w-4 text-[var(--text-muted)] transition-transform duration-200"
                  :class="summaryExpanded ? 'rotate-180' : ''"
                />
              </span>
            </button>
          </h2>
          <button
            v-if="summary && summary.failed && summaryExpanded"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
            :class="
              summaryLoading
                ? 'cursor-wait bg-[var(--bg-secondary)] text-[var(--text-muted)]'
                : 'bg-[var(--focus-ring)] text-white hover:opacity-90'
            "
            :disabled="summaryLoading"
            @click="generateSummary"
          >
            <Loader2 v-if="summaryLoading" class="h-3 w-3 animate-spin" />
            {{ summaryLoading ? "Generating..." : "Retry" }}
          </button>
        </div>

        <div v-if="summaryExpanded" class="mt-4">
          <div v-if="summaryError" class="mb-3">
            <ErrorBanner :message="summaryError" />
          </div>

          <div v-if="summary && summary.available">
            <MarkdownRenderer :content="summary.summary" />
          </div>
          <div
            v-else-if="summary && summary.failed"
            class="rounded-md border border-dashed border-[var(--border-secondary)] p-6 text-center"
          >
            <Sparkles class="mx-auto mb-2 h-6 w-6 text-[var(--text-muted)]" />
            <p class="text-sm text-[var(--text-secondary)]">Summary Generation Failed</p>
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              {{ summary.error || "Click retry to try again." }}
            </p>
          </div>
          <div
            v-else-if="summary && !summary.available"
            class="rounded-md border border-dashed border-[var(--border-secondary)] p-6 text-center"
          >
            <Sparkles class="mx-auto mb-2 h-6 w-6 text-[var(--text-muted)]" />
            <p class="text-sm text-[var(--text-secondary)]">AI Summary Unavailable</p>
            <p class="mt-1 text-xs text-[var(--text-muted)]">
              Configure MEMORY_LLM_URL in the backend to enable AI-powered daily summaries.
            </p>
          </div>
          <div v-else class="py-4 text-center text-sm text-[var(--text-muted)]">
            No daily summary available
          </div>
        </div>
      </Card>

      <div class="rise [animation-delay:120ms]" :class="chartGridClass">
        <Card>
          <h2 class="eyebrow mb-3 flex items-center gap-2">
            <Activity class="h-4 w-4" />
            Incident Trend
          </h2>
          <div v-if="incidentTrendChartData" class="relative h-48 w-full">
            <Line
              :data="incidentTrendChartData"
              :options="lineChartOptions"
              class="!h-full !w-full"
            />
          </div>
          <div
            v-else
            class="flex h-48 items-center justify-center text-sm text-[var(--text-muted)]"
          >
            No trend data available
          </div>
        </Card>

        <Card>
          <h2 class="eyebrow mb-3 flex items-center gap-2">
            <ShieldAlert class="h-4 w-4" />
            Severity Distribution
          </h2>
          <div v-if="severityChartData" class="relative h-48 w-full">
            <Doughnut
              :data="severityChartData"
              :options="doughnutChartOptions"
              class="!h-full !w-full"
            />
          </div>
          <div
            v-else
            class="flex h-48 items-center justify-center text-sm text-[var(--text-muted)]"
          >
            No severity data available
          </div>
        </Card>

        <Card v-if="stats.incidents.by_priority">
          <h2 class="eyebrow mb-3 flex items-center gap-2">
            <ShieldAlert class="h-4 w-4" />
            Incident Priority
          </h2>
          <div v-if="incidentPriorityChartData" class="relative h-48 w-full">
            <Doughnut
              :data="incidentPriorityChartData"
              :options="doughnutChartOptions"
              class="!h-full !w-full"
            />
          </div>
          <div
            v-else
            class="flex h-48 items-center justify-center text-sm text-[var(--text-muted)]"
          >
            No priority data
          </div>
        </Card>

        <Card>
          <h2 class="eyebrow mb-3 flex items-center gap-2">
            <BarChart3 class="h-4 w-4" />
            Top Services by Incidents
          </h2>
          <div v-if="topServicesChartData" class="relative h-48 w-full">
            <Bar :data="topServicesChartData" :options="barChartOptions" class="!h-full !w-full" />
          </div>
          <div
            v-else
            class="flex h-48 items-center justify-center text-sm text-[var(--text-muted)]"
          >
            No service data available
          </div>
        </Card>

        <Card>
          <h2 class="eyebrow mb-3 flex items-center gap-2">
            <TrendingUp class="h-4 w-4" />
            MTTA / MTTR Trend
          </h2>
          <div v-if="mttaMttrChartData" class="relative h-48 w-full">
            <Line :data="mttaMttrChartData" :options="lineChartOptions" class="!h-full !w-full" />
          </div>
          <div
            v-else
            class="flex h-48 items-center justify-center text-sm text-[var(--text-muted)]"
          >
            No trend data available
          </div>
        </Card>
      </div>

      <div class="rise grid grid-cols-1 gap-4 md:grid-cols-3 [animation-delay:180ms]">
        <Card class="sm:col-span-1 lg:col-span-1">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="eyebrow flex items-center gap-2">
              <AlertTriangle class="h-4 w-4" />
              Active
            </h2>
            <button
              class="cursor-pointer text-xs text-[var(--text-link)] transition-colors hover:text-[var(--text-link-hover)]"
              @click="navigateToIncidents({ status: 'active' })"
            >
              View all
            </button>
          </div>

          <div
            v-if="stats.active_incidents?.length > 0 || stats.active_investigations.length > 0"
            class="space-y-1.5"
          >
            <div
              v-for="inc in (stats.active_incidents ?? []).slice(0, 3)"
              :key="inc.incident_number"
              class="flex cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 transition-colors hover:bg-[var(--bg-secondary)]"
              role="button"
              tabindex="0"
              @click="router.push({ path: `/incidents/${inc.incident_number}` })"
              @keydown="
                handleCardKeydown($event, () =>
                  router.push({ path: `/incidents/${inc.incident_number}` }),
                )
              "
            >
              <div
                class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[10px] font-bold"
                :class="incidentPriorityBadgeClass(inc.priority)"
              >
                {{ inc.priority }}
              </div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium text-[var(--text-primary)]">
                  #{{ inc.incident_number }} &middot; {{ inc.title }}
                </div>
                <div class="flex items-center gap-2 text-xs text-[var(--text-muted)]">
                  <span
                    class="inline-flex rounded-full px-1.5 py-0.5"
                    :class="incidentStatusBadgeClass(inc.status)"
                    >{{ incidentStatusLabel(inc.status) }}</span
                  >
                  <span v-if="inc.service_name">{{ inc.service_name }}</span>
                </div>
              </div>
              <span class="shrink-0 text-[11px] text-[var(--text-muted)]">{{
                formatTimeAgo(inc.created_at)
              }}</span>
            </div>

            <div
              v-for="inv in stats.active_investigations.slice(0, 3)"
              :key="inv.investigation_id"
              class="flex items-center gap-2.5 rounded-md px-2.5 py-2"
            >
              <div
                class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full"
                :class="investigationStatusBadgeClass(inv.status)"
              >
                <Bot v-if="inv.agent_name" class="h-3 w-3" />
                <Search v-else class="h-3 w-3" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium text-[var(--text-primary)]">
                  {{ inv.alert_name || inv.correlation_key }}
                </div>
                <div class="flex items-center gap-2 text-xs text-[var(--text-muted)]">
                  <span
                    class="inline-flex rounded-full px-1.5 py-0.5"
                    :class="investigationStatusBadgeClass(inv.status)"
                    >{{ investigationStatusLabel(inv.status) }}</span
                  >
                  <span v-if="inv.agent_name" class="flex items-center gap-1"
                    ><Bot class="h-3 w-3" />{{ inv.agent_name }}</span
                  >
                </div>
              </div>
              <span class="shrink-0 text-[11px] text-[var(--text-muted)]">{{
                formatTimeAgo(inv.created_at)
              }}</span>
            </div>
          </div>
          <div v-else class="py-6 text-center text-sm text-[var(--text-muted)]">
            No active incidents or investigations
          </div>
        </Card>

        <Card class="sm:col-span-1 lg:col-span-1">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="eyebrow flex items-center gap-2">
              <Server class="h-4 w-4" />
              Service Health
            </h2>
            <button
              class="cursor-pointer text-xs text-[var(--text-link)] transition-colors hover:text-[var(--text-link-hover)]"
              @click="router.push({ path: '/services' })"
            >
              View all
            </button>
          </div>

          <div v-if="services.length > 0" class="space-y-1.5">
            <div
              v-for="svc in nonOperationalServices.slice(0, 5)"
              :key="svc.id"
              class="flex cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 transition-colors hover:bg-[var(--bg-secondary)]"
              role="button"
              tabindex="0"
              @click="router.push({ path: `/services/${svc.id}` })"
              @keydown="
                handleCardKeydown($event, () => router.push({ path: `/services/${svc.id}` }))
              "
            >
              <div
                class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[10px]"
                :class="serviceStatusBadgeClass(svc.status)"
              ></div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium text-[var(--text-primary)]">
                  {{ svc.display_name || svc.name }}
                </div>
                <div class="text-xs text-[var(--text-muted)]">
                  {{ serviceStatusLabel(svc.status) }}
                </div>
              </div>
              <span v-if="svc.active_incident_count" class="text-xs text-[var(--text-badge-firing)]"
                >{{ svc.active_incident_count }} incident{{
                  svc.active_incident_count > 1 ? "s" : ""
                }}</span
              >
            </div>
            <div
              v-for="svc in operationalServices.slice(
                0,
                5 - Math.min(nonOperationalServices.length, 5),
              )"
              :key="svc.id"
              class="flex cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 transition-colors hover:bg-[var(--bg-secondary)]"
              role="button"
              tabindex="0"
              @click="router.push({ path: `/services/${svc.id}` })"
              @keydown="
                handleCardKeydown($event, () => router.push({ path: `/services/${svc.id}` }))
              "
            >
              <div
                class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[10px]"
                :class="serviceStatusBadgeClass(svc.status)"
              ></div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm text-[var(--text-secondary)]">
                  {{ svc.display_name || svc.name }}
                </div>
              </div>
            </div>
          </div>
          <div v-else class="py-6 text-center text-sm text-[var(--text-muted)]">
            No services configured
          </div>
        </Card>

        <Card class="sm:col-span-1 lg:col-span-1">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="eyebrow flex items-center gap-2">
              <Radio class="h-4 w-4" />
              On-Call Now
            </h2>
            <button
              class="cursor-pointer text-xs text-[var(--text-link)] transition-colors hover:text-[var(--text-link-hover)]"
              @click="router.push({ path: '/on-call' })"
            >
              Schedules
            </button>
          </div>

          <div v-if="onCallEntries.length > 0" class="space-y-1.5">
            <div
              v-for="entry in onCallEntries"
              :key="entry.schedule_id + entry.user_id"
              class="flex items-center gap-2.5 rounded-md px-2.5 py-2"
            >
              <div
                class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[var(--focus-ring)]/15 text-xs font-bold text-[var(--focus-ring)]"
              >
                {{
                  (entry.user_display_name ?? "?")
                    .split(" ")
                    .map((w: string) => w[0])
                    .join("")
                    .slice(0, 2)
                    .toUpperCase()
                }}
              </div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium text-[var(--text-primary)]">
                  {{ entry.user_display_name ?? "Unknown" }}
                </div>
                <div class="text-xs text-[var(--text-muted)]">{{ entry.schedule_name }}</div>
              </div>
              <span v-if="entry.until" class="shrink-0 text-[11px] text-[var(--text-muted)]">
                {{ formatDurationFromMs(new Date(entry.until).getTime() - Date.now()) }}
              </span>
            </div>
          </div>
          <div v-else class="py-6 text-center text-sm text-[var(--text-muted)]">
            No one currently on call
          </div>
        </Card>
      </div>

      <Card class="rise [animation-delay:220ms]">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-4">
          <div class="text-sm font-medium text-[var(--text-secondary)]">
            Investigations
            <span class="ml-1 text-xs text-[var(--text-muted)]">
              {{ stats.investigations.total }} total &middot;
              {{ stats.investigations.completion_rate.toFixed(1) }}% complete
            </span>
          </div>
          <div class="flex h-1.5 flex-1 items-center gap-0.5 overflow-hidden rounded-full">
            <div
              v-for="seg in investigationBreakdown"
              :key="seg.label"
              :class="seg.cls"
              :style="{ width: seg.pct + '%', minWidth: seg.value > 0 ? '2px' : '0' }"
              :title="`${seg.label}: ${seg.value} (${seg.pct}%)`"
              class="h-full first:rounded-l-full last:rounded-r-full transition-all duration-300"
            ></div>
          </div>
          <div class="hidden items-center gap-3 text-xs text-[var(--text-muted)] sm:flex">
            <span
              v-for="seg in investigationBreakdown"
              :key="seg.label"
              class="flex items-center gap-1"
            >
              <span class="inline-block h-2 w-2 rounded-full" :class="seg.cls"></span>
              {{ seg.value }} {{ seg.label }}
            </span>
          </div>
        </div>
      </Card>

      <div v-if="recentActivity.length > 0" class="rise [animation-delay:260ms]">
        <Card>
          <div class="mb-3 flex items-center justify-between">
            <h2 class="eyebrow">Recent Activity</h2>
          </div>
          <div class="divide-y divide-[var(--border-primary)]">
            <div
              v-for="item in recentActivity"
              :key="item.key"
              :class="[
                'flex items-center gap-3 px-1 py-2.5 first:pt-0 last:pb-0',
                item.route ? 'cursor-pointer transition-colors hover:bg-[var(--bg-secondary)]' : '',
              ]"
              role="button"
              :tabindex="item.route ? 0 : -1"
              @click="item.route && router.push({ path: item.route })"
              @keydown="
                handleCardKeydown($event, () => item.route && router.push({ path: item.route }))
              "
            >
              <div
                class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs"
                :class="
                  item.type === 'incident'
                    ? 'bg-[var(--text-badge-firing)]/15 text-[var(--text-badge-firing)]'
                    : 'bg-[var(--focus-ring)]/15 text-[var(--focus-ring)]'
                "
              >
                <AlertOctagon v-if="item.type === 'incident'" class="h-3.5 w-3.5" />
                <Bot v-else class="h-3.5 w-3.5" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium text-[var(--text-primary)]">
                  {{ item.title }}
                </div>
                <div
                  class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--text-muted)]"
                >
                  <span v-if="item.subtitle">{{ item.subtitle }}</span>
                  <span
                    v-if="item.subtitle"
                    class="hidden sm:inline-block h-1 w-1 rounded-full bg-[var(--text-muted)]"
                  ></span>
                  <span
                    class="inline-flex rounded-full px-1.5 py-0.5"
                    :class="item.statusBadgeClass"
                    >{{ item.statusLabel }}</span
                  >
                  <span class="flex items-center gap-1 sm:hidden">
                    <Clock class="h-3 w-3" />
                    {{ formatTimeAgo(item.created_at) }}
                  </span>
                </div>
              </div>
              <span
                class="hidden shrink-0 items-center gap-1 whitespace-nowrap text-[11px] text-[var(--text-muted)] sm:flex"
              >
                <Clock class="h-3 w-3" />
                {{ formatTimeAgo(item.created_at) }}
              </span>
              <ChevronRight class="h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]" />
            </div>
          </div>
        </Card>
      </div>
    </template>
  </div>
</template>
