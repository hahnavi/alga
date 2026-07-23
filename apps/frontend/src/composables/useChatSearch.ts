import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from "vue";
import {
  focusHeaderSearchOwner,
  headerSearchActive,
  headerSearchState,
  registerHeaderSearchOwner,
  unregisterHeaderSearchOwner,
} from "@/lib/pageHeader";

export function useChatSearch<T>(
  items: Ref<T[]>,
  getId: (item: T) => string,
  getText: (item: T) => string,
) {
  const query = ref("");
  const active = ref(false);
  const currentIndex = ref(0);

  const lowerQuery = computed(() => query.value.toLowerCase().trim());

  const matchIds = computed(() => {
    const q = lowerQuery.value;
    if (!q) return [];
    const ids: string[] = [];
    for (const item of items.value) {
      if (getText(item).toLowerCase().includes(q)) {
        ids.push(getId(item));
      }
    }
    return ids;
  });

  const matchCount = computed(() => matchIds.value.length);
  const currentMatchId = computed(() => matchIds.value[currentIndex.value] ?? null);
  const hasQuery = computed(() => lowerQuery.value.length > 0);

  function isMatch(id: string): boolean {
    if (!hasQuery.value) return false;
    return matchIds.value.includes(id);
  }

  function isCurrentMatch(id: string): boolean {
    return currentMatchId.value === id;
  }

  function scrollToCurrentMatch() {
    const targetId = currentMatchId.value;
    if (!targetId) return;
    nextTick(() => {
      const el = document.querySelector(`[data-chat-msg-id="${CSS.escape(targetId)}"]`);
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "center" });
      }
    });
  }

  function nextMatch() {
    if (matchCount.value === 0) return;
    currentIndex.value = (currentIndex.value + 1) % matchCount.value;
    scrollToCurrentMatch();
  }

  function prevMatch() {
    if (matchCount.value === 0) return;
    currentIndex.value = (currentIndex.value - 1 + matchCount.value) % matchCount.value;
    scrollToCurrentMatch();
  }

  const ownerId = registerHeaderSearchOwner({
    onUpdateQuery: (v: string) => {
      query.value = v;
    },
    onNext: nextMatch,
    onPrev: prevMatch,
    onClose: closeSearch,
  });

  function syncHeaderState() {
    headerSearchState.query = query.value;
    headerSearchState.matchCount = matchCount.value;
    headerSearchState.currentIndex = currentIndex.value;
  }

  function openSearch() {
    active.value = true;
    headerSearchActive.value = true;
    focusHeaderSearchOwner(ownerId);
    syncHeaderState();
  }

  function closeSearch() {
    active.value = false;
    headerSearchActive.value = false;
    query.value = "";
    currentIndex.value = 0;
    syncHeaderState();
  }

  watch([query, matchCount, currentIndex], () => {
    if (active.value) syncHeaderState();
  });

  watch(query, () => {
    currentIndex.value = 0;
    if (matchCount.value > 0) {
      scrollToCurrentMatch();
    }
  });

  function handleKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key === "f") {
      e.preventDefault();
      openSearch();
      return;
    }
    if (e.key === "Escape" && active.value) {
      e.preventDefault();
      closeSearch();
      return;
    }
    if (active.value && e.key === "Enter") {
      e.preventDefault();
      if (e.shiftKey) prevMatch();
      else nextMatch();
    }
  }

  onMounted(() => {
    document.addEventListener("keydown", handleKeydown);
  });

  onBeforeUnmount(() => {
    document.removeEventListener("keydown", handleKeydown);
    if (active.value) {
      headerSearchActive.value = false;
    }
    unregisterHeaderSearchOwner(ownerId);
  });

  function searchHighlight(id: string): string {
    if (!hasQuery.value) return "";
    if (isCurrentMatch(id))
      return "ring-2 ring-inset ring-[var(--accent)/0.7] bg-[var(--accent)/0.05]";
    if (isMatch(id)) return "ring-1 ring-inset ring-[var(--accent)/0.4]";
    return "opacity-40";
  }

  return {
    query,
    active,
    matchCount,
    currentIndex,
    currentMatchId,
    hasQuery,
    isMatch,
    isCurrentMatch,
    searchHighlight,
    nextMatch,
    prevMatch,
    openSearch,
    closeSearch,
  };
}
