<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { FileText, CheckCircle2, AlertTriangle, ChevronRight } from "@lucide/vue";
import { api, type PostMortemRecord } from "@/lib/api";
import { postMortemStatusBadgeClass } from "@/lib/alertLabels";
import { formatTimeAgo } from "@/lib/time";
import { usePageHeaderActions } from "@/composables/usePageHeaderActions";
import SeverityBadge from "@/components/ui/SeverityBadge.vue";
import Select from "@/components/ui/Select.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import InteractiveCard from "@/components/ui/InteractiveCard.vue";
import { useListPage } from "@/composables/useListPage";
import { useListFilter } from "@/composables/useListFilter";

defineOptions({ name: "PostMortemsPage" });

const router = useRouter();

const statusFilter = ref("");
const searchInput = ref("");

const {
  items: postMortems,
  loading,
  error,
  reload: load,
} = useListPage<PostMortemRecord>({
  fetch: () => {
    const params: Record<string, string | number | boolean | undefined> = { limit: 100 };
    if (statusFilter.value) params.status = statusFilter.value;
    return api.listPostMortems(params);
  },
  entityName: "post-mortems",
});
function statusLabel(status: string): string {
  return status.replace("_", " ");
}

function openActionItemCount(pm: PostMortemRecord): number {
  return (pm.action_items ?? []).filter((a) => a.status === "open" || a.status === "in_progress")
    .length;
}

usePageHeaderActions({
  title: "Post-Mortems",
  titleIcon: FileText,
  searchInput,
  searchPlaceholder: "Search post-mortems...",
  showAdd: false,
});

function navigateToPostMortem(pm: PostMortemRecord) {
  router.push(`/incidents/${pm.incident_number}/post-mortem`);
}

function navigateToIncident(pm: PostMortemRecord) {
  router.push(`/incidents/${pm.incident_number}`);
}

const filteredPostMortems = useListFilter(
  postMortems,
  ["summary", "root_cause", "impact", "incident_title"],
  searchInput,
);

watch(statusFilter, () => load());

onMounted(() => {
  load();
});
</script>

<template>
  <section class="space-y-4 px-4 py-4 md:space-y-6 md:px-6 md:py-6">
    <div class="flex flex-wrap items-center gap-3">
      <Select v-model="statusFilter" class="w-36">
        <option value="">All statuses</option>
        <option value="draft">Draft</option>
        <option value="in_review">In Review</option>
        <option value="approved">Approved</option>
        <option value="published">Published</option>
      </Select>
      <span class="text-sm text-[var(--text-muted)]">
        {{ filteredPostMortems.length }}
        {{ filteredPostMortems.length === 1 ? "post-mortem" : "post-mortems" }}
      </span>
    </div>

    <ErrorBanner v-if="error" :message="error" @dismiss="error = ''" />

    <LoadingSpinner v-if="loading && postMortems.length === 0" />

    <EmptyState
      v-else-if="filteredPostMortems.length === 0"
      :message="
        searchInput
          ? 'No post-mortems match your search.'
          : 'No post-mortems have been created yet.'
      "
    >
      <template #icon>
        <FileText class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>

    <div v-else class="space-y-3">
      <InteractiveCard
        v-for="pm in filteredPostMortems"
        :key="pm.id"
        @click="navigateToPostMortem(pm)"
      >
        <div class="flex items-start gap-3">
          <div
            class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-secondary)]"
          >
            <FileText class="h-4 w-4 text-[var(--text-muted)]" />
          </div>

          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-medium text-[var(--text-primary)]">
                {{ pm.incident_title || `Incident #${pm.incident_number}` }}
              </span>
              <span :class="['badge', postMortemStatusBadgeClass(pm.status)]">
                {{ statusLabel(pm.status) }}
              </span>
              <SeverityBadge
                v-if="pm.incident_severity"
                :severity="pm.incident_severity"
                class="text-[10px]"
              />
            </div>

            <p v-if="pm.summary" class="mt-1 line-clamp-2 text-sm text-[var(--text-secondary)]">
              {{ pm.summary }}
            </p>

            <div
              class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]"
            >
              <span>{{ formatTimeAgo(pm.created_at) }}</span>
              <span
                v-if="pm.action_items && pm.action_items.length > 0"
                class="flex items-center gap-1"
              >
                <CheckCircle2 class="h-3 w-3" />
                {{ openActionItemCount(pm) }} open / {{ pm.action_items.length }} total
              </span>
              <button
                class="inline-flex items-center gap-0.5 text-[var(--text-muted)] transition-colors hover:text-[var(--text-primary)]"
                @click.stop="navigateToIncident(pm)"
              >
                <AlertTriangle class="h-3 w-3" />
                View incident
                <ChevronRight class="h-2.5 w-2.5" />
              </button>
            </div>
          </div>
        </div>
      </InteractiveCard>
    </div>
  </section>
</template>
