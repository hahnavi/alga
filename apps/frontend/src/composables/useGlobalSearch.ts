import { computed, nextTick, ref } from "vue";
import { useRouter, type Router } from "vue-router";
import { api, type AlertRecord, type IncidentRecord } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import type { Tab } from "@/components/ui/Tabs.vue";

export type SearchTab = "alerts" | "incidents";

// Module-level singleton state. Mirrors the `pageHeader` / `headerSearchState`
// pattern: `App.vue` mounts the overlay + Cmd-K listener once and every
// authenticated page shares the same refs, so search is available app-wide.
const searchActive = ref(false);
const searchQuery = ref("");
const searchTab = ref<SearchTab>("alerts");
const searchAlertResults = ref<AlertRecord[]>([]);
const searchIncidentResults = ref<IncidentRecord[]>([]);
const searchLoading = ref(false);
const searchError = ref("");
const searchFocusedIndex = ref(-1);
const searchSubmitted = ref(false);
let searchSeq = 0;

// Captured from the first `useGlobalSearch()` call (always `App.vue` setup,
// before any user interaction). Stored at module scope so the activation
// handlers can route without re-entering setup.
let router: Router | null = null;

function openGlobalSearch() {
  searchActive.value = true;
  nextTick(() => {
    const input = document.querySelector<HTMLInputElement>("[data-global-search-input]");
    input?.focus();
  });
}

function closeGlobalSearch() {
  searchActive.value = false;
  searchSubmitted.value = false;
  searchFocusedIndex.value = -1;
  searchAlertResults.value = [];
  searchIncidentResults.value = [];
  searchError.value = "";
  searchLoading.value = false;
  searchSeq++;
}

function clearSearchQuery() {
  searchSeq++;
  searchQuery.value = "";
  searchSubmitted.value = false;
  searchFocusedIndex.value = -1;
  searchAlertResults.value = [];
  searchIncidentResults.value = [];
}

async function executeGlobalSearch(query: string) {
  if (!query.trim()) {
    searchAlertResults.value = [];
    searchIncidentResults.value = [];
    searchFocusedIndex.value = -1;
    return;
  }
  const seq = ++searchSeq;
  searchLoading.value = true;
  searchError.value = "";
  searchFocusedIndex.value = -1;
  try {
    const [alerts, incs] = await Promise.all([
      api.searchAlerts(query, 10).catch(() => null),
      api.searchIncidents(query, 10).catch(() => null),
    ]);
    if (seq !== searchSeq) return;
    searchAlertResults.value = alerts ?? [];
    searchIncidentResults.value = incs?.items ?? [];
    const counts = {
      alerts: (alerts ?? []).length,
      incidents: incs?.items?.length ?? 0,
    };
    if (counts[searchTab.value] === 0) {
      const best = (Object.entries(counts) as [SearchTab, number][]).sort((a, b) => b[1] - a[1])[0];
      if (best[1] > 0) searchTab.value = best[0];
    }
  } catch (e: unknown) {
    if (seq !== searchSeq) return;
    searchError.value = getErrorMessage(e, "Search failed");
  } finally {
    if (seq === searchSeq) {
      searchLoading.value = false;
    }
  }
}

function submitSearch() {
  const q = searchQuery.value.trim();
  if (!q) {
    searchAlertResults.value = [];
    searchIncidentResults.value = [];
    searchSubmitted.value = false;
    searchFocusedIndex.value = -1;
    return;
  }
  searchSubmitted.value = true;
  void executeGlobalSearch(q);
}

function handleGlobalSearchKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
    e.preventDefault();
    if (searchActive.value) closeGlobalSearch();
    else openGlobalSearch();
  }
}

const searchTabCounts = computed(() => ({
  alerts: searchAlertResults.value.length,
  incidents: searchIncidentResults.value.length,
}));

const searchTabItems = computed<Tab<SearchTab>[]>(() => [
  { id: "alerts", label: "Alerts", count: searchTabCounts.value.alerts },
  { id: "incidents", label: "Incidents", count: searchTabCounts.value.incidents },
]);

const currentSearchResults = computed(() => {
  if (searchTab.value === "alerts") return searchAlertResults.value;
  return searchIncidentResults.value;
});

const searchTotalCount = computed(
  () => searchTabCounts.value.alerts + searchTabCounts.value.incidents,
);

function handleSearchInputKeydown(e: KeyboardEvent) {
  const count = currentSearchResults.value.length;
  if (e.key === "ArrowDown") {
    e.preventDefault();
    searchFocusedIndex.value = count > 0 ? Math.min(searchFocusedIndex.value + 1, count - 1) : -1;
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    searchFocusedIndex.value = count > 0 ? Math.max(searchFocusedIndex.value - 1, 0) : -1;
  } else if (e.key === "Enter") {
    if (searchFocusedIndex.value >= 0) {
      e.preventDefault();
      activateSearchResult(searchFocusedIndex.value);
    } else {
      submitSearch();
    }
  }
}

function activateSearchResult(index: number) {
  const results = currentSearchResults.value;
  if (index < 0 || index >= results.length) return;
  const item = results[index];
  if (!router) return;
  if (searchTab.value === "alerts") {
    const alert = item as AlertRecord;
    router.push({
      path: alert.alert_number ? `/alerts/${alert.alert_number}` : `/alerts/${alert.fingerprint}`,
    });
  } else {
    router.push({ path: `/incidents/${(item as IncidentRecord).incident_number}` });
  }
  closeGlobalSearch();
}

/**
 * Global Cmd-K search overlay. Module-level singleton: `App.vue` mounts the
 * overlay + registers the Cmd-K key listener once; any authenticated page can
 * trigger search via the same shared state (e.g. a header search button).
 */
export function useGlobalSearch() {
  router = useRouter();
  return {
    searchActive,
    searchQuery,
    searchTab,
    searchAlertResults,
    searchIncidentResults,
    searchLoading,
    searchError,
    searchFocusedIndex,
    searchSubmitted,
    openGlobalSearch,
    closeGlobalSearch,
    clearSearchQuery,
    executeGlobalSearch,
    submitSearch,
    handleGlobalSearchKeydown,
    handleSearchInputKeydown,
    activateSearchResult,
    searchTabCounts,
    searchTabItems,
    currentSearchResults,
    searchTotalCount,
  };
}
