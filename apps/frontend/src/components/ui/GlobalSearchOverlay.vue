<script setup lang="ts">
import { Search, X, FileSearch, Zap, ArrowUpRight, Clock, Loader2 } from "@lucide/vue";
import Tabs from "@/components/ui/Tabs.vue";
import AlertStatusBadge from "@/components/ui/AlertStatusBadge.vue";
import { useGlobalSearch } from "@/composables/useGlobalSearch";
import { useEscapeKey } from "@/composables/useEscapeKey";
import {
  severityBadgeClass,
  incidentPriorityBadgeClass,
  incidentStatusBadgeClass,
  incidentStatusLabel,
} from "@/lib/alertLabels";
import { formatTimeAgo } from "@/lib/time";

defineOptions({ name: "GlobalSearchOverlay" });

const {
  searchActive,
  searchQuery,
  searchTab,
  searchAlertResults,
  searchIncidentResults,
  searchLoading,
  searchError,
  searchFocusedIndex,
  searchSubmitted,
  closeGlobalSearch,
  clearSearchQuery,
  handleSearchInputKeydown,
  activateSearchResult,
  searchTabItems,
  searchTotalCount,
} = useGlobalSearch();

useEscapeKey(
  () => closeGlobalSearch(),
  () => searchActive.value,
);
</script>

<template>
  <Teleport to="body">
    <Transition name="search-overlay">
      <div
        v-if="searchActive"
        class="search-backdrop fixed inset-0 z-50 flex items-start justify-center px-4 pt-[8vh] sm:pt-[12vh]"
        @click.self="closeGlobalSearch"
      >
        <div
          class="search-dialog relative flex max-h-[75vh] w-full max-w-2xl flex-col overflow-hidden rounded border border-[var(--border-primary)] bg-[var(--bg-dialog)] text-[var(--text-primary)] shadow-2xl"
        >
          <div class="flex items-center gap-3 border-b border-[var(--border-primary)] px-4 py-3">
            <Search class="h-5 w-5 shrink-0 text-[var(--text-muted)]" />
            <input
              data-global-search-input
              :value="searchQuery"
              type="text"
              placeholder="Search alerts and incidents..."
              class="min-w-0 flex-1 bg-transparent text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] outline-none"
              @input="searchQuery = ($event.target as HTMLInputElement).value"
              @keydown="handleSearchInputKeydown"
              @keydown.escape.prevent="closeGlobalSearch"
            />
            <button
              v-if="searchQuery"
              type="button"
              class="flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
              @click="clearSearchQuery"
            >
              <X class="h-3.5 w-3.5" />
            </button>
            <kbd
              class="hidden shrink-0 rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--text-muted)] sm:inline-block"
              >ESC</kbd
            >
          </div>

          <Tabs
            v-if="searchSubmitted"
            v-model="searchTab"
            :tabs="searchTabItems"
            aria-label="Search results sections"
            id-prefix="global-search"
            @update:model-value="searchFocusedIndex = -1"
          />

          <div class="flex-1 overflow-y-auto overscroll-contain">
            <div v-if="searchLoading" class="flex items-center justify-center py-16">
              <Loader2 class="h-6 w-6 animate-spin text-[var(--text-muted)]" />
            </div>
            <div v-else-if="searchError" class="px-4 py-6">
              <p class="text-center text-sm text-[var(--text-error)]">{{ searchError }}</p>
            </div>
            <div v-else-if="searchSubmitted">
              <div v-if="searchTab === 'alerts'">
                <div v-if="searchAlertResults.length > 0">
                  <div
                    v-for="(alert, idx) in searchAlertResults"
                    :key="alert.fingerprint"
                    class="flex cursor-pointer items-center gap-3 px-4 py-3 transition-colors"
                    :class="
                      searchFocusedIndex === idx
                        ? 'bg-[var(--focus-ring)]/8'
                        : 'hover:bg-[var(--bg-secondary)]'
                    "
                    role="option"
                    :aria-selected="searchFocusedIndex === idx"
                    tabindex="-1"
                    @click="activateSearchResult(idx)"
                    @mouseenter="searchFocusedIndex = idx"
                  >
                    <div
                      class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-xs font-bold"
                      :class="severityBadgeClass(alert.labels?.severity ?? 'unknown')"
                    >
                      {{ (alert.labels?.severity ?? "?")[0].toUpperCase() }}
                    </div>
                    <div class="min-w-0 flex-1">
                      <div class="truncate text-sm font-medium text-[var(--text-primary)]">
                        <template v-if="alert.alert_number"
                          >#{{ alert.alert_number }} &middot; </template
                        >{{ (alert.labels?.alertname ?? "Unknown alert").trim() }}
                      </div>
                      <div class="flex items-center gap-2 text-xs text-[var(--text-muted)]">
                        <span v-if="alert.labels?.namespace">{{ alert.labels.namespace }}</span>
                        <span
                          v-if="alert.labels?.namespace"
                          class="inline-block h-1 w-1 rounded-full bg-[var(--text-muted)]"
                        ></span>
                        <span class="inline-flex items-center rounded-full">
                          <AlertStatusBadge
                            :status="alert.status"
                            :acknowledged="alert.acknowledged"
                          />
                        </span>
                      </div>
                    </div>
                    <span
                      class="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs text-[var(--text-muted)]"
                    >
                      <Clock class="h-3 w-3" />
                      {{ formatTimeAgo(alert.created_at) }}
                    </span>
                    <ArrowUpRight
                      v-if="searchFocusedIndex === idx"
                      class="h-3.5 w-3.5 shrink-0 text-[var(--focus-ring)]"
                    />
                  </div>
                </div>
                <div v-else class="px-4 py-16 text-center">
                  <FileSearch class="mx-auto mb-3 h-8 w-8 text-[var(--text-muted)] opacity-40" />
                  <p class="text-sm text-[var(--text-muted)]">No matching alerts</p>
                </div>
              </div>

              <div v-if="searchTab === 'incidents'">
                <div v-if="searchIncidentResults.length > 0">
                  <div
                    v-for="(inc, idx) in searchIncidentResults"
                    :key="inc.incident_number"
                    class="flex cursor-pointer items-center gap-3 px-4 py-3 transition-colors"
                    :class="
                      searchFocusedIndex === idx
                        ? 'bg-[var(--focus-ring)]/8'
                        : 'hover:bg-[var(--bg-secondary)]'
                    "
                    role="option"
                    :aria-selected="searchFocusedIndex === idx"
                    tabindex="-1"
                    @click="activateSearchResult(idx)"
                    @mouseenter="searchFocusedIndex = idx"
                  >
                    <div
                      class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-xs font-bold"
                      :class="incidentPriorityBadgeClass(inc.priority)"
                    >
                      {{ inc.priority }}
                    </div>
                    <div class="min-w-0 flex-1">
                      <div class="truncate text-sm font-medium text-[var(--text-primary)]">
                        #{{ inc.incident_number }} &middot;
                        {{ inc.title }}
                      </div>
                      <div class="flex items-center gap-2 text-xs text-[var(--text-muted)]">
                        <span
                          class="inline-flex items-center rounded-full px-1.5 py-0.5"
                          :class="incidentStatusBadgeClass(inc.status)"
                          >{{ incidentStatusLabel(inc.status) }}</span
                        >
                      </div>
                    </div>
                    <span
                      class="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs text-[var(--text-muted)]"
                    >
                      <Clock class="h-3 w-3" />
                      {{ formatTimeAgo(inc.created_at) }}
                    </span>
                    <ArrowUpRight
                      v-if="searchFocusedIndex === idx"
                      class="h-3.5 w-3.5 shrink-0 text-[var(--focus-ring)]"
                    />
                  </div>
                </div>
                <div v-else class="px-4 py-16 text-center">
                  <FileSearch class="mx-auto mb-3 h-8 w-8 text-[var(--text-muted)] opacity-40" />
                  <p class="text-sm text-[var(--text-muted)]">No matching incidents</p>
                </div>
              </div>
            </div>

            <div v-else class="px-4 py-16 text-center">
              <Search class="mx-auto mb-3 h-8 w-8 text-[var(--text-muted)] opacity-30" />
              <p class="text-sm text-[var(--text-muted)]">Search across alerts and incidents</p>
              <div
                class="mt-3 flex items-center justify-center gap-4 text-xs text-[var(--text-muted)]"
              >
                <span class="flex items-center gap-1"><Zap class="h-3 w-3" /> Quick search</span>
                <span class="flex items-center gap-1"
                  ><kbd
                    class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-1 py-0.5 text-[10px]"
                    >Ctrl+K</kbd
                  >
                  to open</span
                >
              </div>
            </div>
          </div>

          <div
            v-if="searchSubmitted && searchTotalCount > 0 && !searchLoading"
            class="flex items-center justify-between border-t border-[var(--border-primary)] px-4 py-2 text-[10px] text-[var(--text-muted)]"
          >
            <span>{{ searchTotalCount }} result{{ searchTotalCount !== 1 ? "s" : "" }}</span>
            <span class="flex items-center gap-3">
              <span class="flex items-center gap-1"
                ><kbd
                  class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-1 py-0.5"
                  >&uarr;&darr;</kbd
                >
                navigate</span
              >
              <span class="flex items-center gap-1"
                ><kbd
                  class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-1 py-0.5"
                  >&crarr;</kbd
                >
                open</span
              >
            </span>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.search-backdrop {
  background: rgb(0 0 0 / 0.5);
  backdrop-filter: blur(4px);
  -webkit-backdrop-filter: blur(4px);
}

:root.light .search-backdrop {
  background: rgb(0 0 0 / 0.3);
}

.search-overlay-enter-active {
  transition: opacity 0.15s ease;
}
.search-overlay-enter-active .search-dialog {
  transition:
    opacity 0.15s ease,
    transform 0.15s ease;
}
.search-overlay-leave-active {
  transition: opacity 0.1s ease;
}
.search-overlay-leave-active .search-dialog {
  transition:
    opacity 0.1s ease,
    transform 0.1s ease;
}
.search-overlay-enter-from {
  opacity: 0;
}
.search-overlay-enter-from .search-dialog {
  opacity: 0;
  transform: scale(0.97) translateY(-8px);
}
.search-overlay-leave-to {
  opacity: 0;
}
.search-overlay-leave-to .search-dialog {
  opacity: 0;
  transform: scale(0.97) translateY(-4px);
}
</style>
