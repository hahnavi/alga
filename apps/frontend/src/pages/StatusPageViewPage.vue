<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ArrowLeft, RefreshCw } from "@lucide/vue";
import { api, type StatusPageView } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import { formatTimeAgo } from "@/lib/time";

defineOptions({ name: "StatusPageViewPage" });

const route = useRoute();
const router = useRouter();

const view = ref<StatusPageView | null>(null);
const loading = ref(false);
const error = ref("");

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
        <Card v-for="(inc, i) in view.incidents" :key="`${inc.title}-${i}`">
          <div class="space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span :class="['badge', incidentStatusLabel(inc.status).cls]">
                {{ incidentStatusLabel(inc.status).text }}
              </span>
              <span class="badge badge-muted">{{ inc.severity }}</span>
              <span v-if="inc.title" class="font-medium">{{ inc.title }}</span>
            </div>
            <div v-if="inc.started_at" class="text-xs text-[var(--text-muted)]">
              Started {{ formatTimeAgo(inc.started_at) }}
            </div>
          </div>
        </Card>
      </div>

      <div class="space-y-2">
        <h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--text-muted)]">
          Components
        </h2>

        <!--
 Read-only by design: the slug view payload is an allow-listed,
          public-safe contract without internal ids, so component management
          (add / status / delete) lives on the Status Pages settings page.
        -->
        <Card v-for="c in view.components" :key="`${c.name}-${c.display_order}`">
          <div class="flex flex-wrap items-center gap-3">
            <span :class="['h-2.5 w-2.5 rounded-full', statusMeta(c.status).dot]" />
            <span class="font-medium">{{ c.name }}</span>
            <span :class="['badge', statusMeta(c.status).badge]">{{
              c.status.replace("_", " ")
            }}</span>
          </div>
          <p v-if="c.description" class="mt-1 text-sm text-[var(--text-secondary)]">
            {{ c.description }}
          </p>
        </Card>

        <EmptyState v-if="view.components.length === 0" message="No components yet." />
      </div>
    </template>
  </section>
</template>
