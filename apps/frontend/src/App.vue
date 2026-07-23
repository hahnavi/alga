<script setup lang="ts">
import { onMounted, onBeforeUnmount, watch, computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Bot, ChevronLeft, Menu, Loader2 } from "@lucide/vue";
import Sidebar from "@/components/Sidebar.vue";
import MobileMoreMenu from "@/components/MobileMoreMenu.vue";
import MobileAgentMenu from "@/components/MobileAgentMenu.vue";
import GlobalSearchOverlay from "@/components/ui/GlobalSearchOverlay.vue";
import SettingsDialog from "@/components/ui/settings/SettingsDialog.vue";
import ToastStack from "@/components/ui/ToastStack.vue";
import RouteLoadingBar from "@/components/ui/RouteLoadingBar.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import { useAuthStore } from "@/stores/auth";
import { useNotificationStore } from "@/stores/notifications";
import { pageHeader, headerSearchActive, headerSearchState } from "@/lib/pageHeader";
import ChatSearchBar from "@/components/ui/ChatSearchBar.vue";
import { useAuthBootstrap } from "@/composables/useAuthBootstrap";
import { useSessionKeepAlive } from "@/composables/useSessionKeepAlive";
import { useSettingsDialogFromQuery } from "@/composables/useSettingsDialogFromQuery";
import { useSSE } from "@/composables/useSSE";
import { isActiveRoute } from "@/lib/routing";
import { setAuthRedirectRouter } from "@/lib/authRedirect";
import { pageTitleForPath } from "@/lib/pageTitles";
import { getAgentBrandIconSrc } from "@/lib/agentAvatar";
import { CARD_ICON_BTN_CLASS } from "@/lib/uiClasses";
import { useRoutePrefetch } from "@/composables/useRoutePrefetch";
import { useGlobalSearch } from "@/composables/useGlobalSearch";
import { useNavSections, TOP_NAV_ITEMS } from "@/lib/nav";

const auth = useAuthStore();
const notificationStore = useNotificationStore();
const { prefetch } = useRoutePrefetch();
useSessionKeepAlive();
const { handleGlobalSearchKeydown, closeGlobalSearch } = useGlobalSearch();
const notificationSSE = useSSE("/api/v1/events", {
  notification_new: (data: unknown) => {
    notificationStore.handleSSEEvent("notification_new", data);
  },
  notification_unread_count: (data: unknown) => {
    notificationStore.handleSSEEvent("notification_unread_count", data);
  },
});
const route = useRoute();
const router = useRouter();
setAuthRedirectRouter(router);

useAuthBootstrap();
const { showSettings, settingsTab, openSettings, closeSettings } = useSettingsDialogFromQuery();

const moreMenuOpen = ref(false);
const agentMenuOpen = ref(false);

const { mobileMorePaths } = useNavSections();
const navItems = TOP_NAV_ITEMS;

const agentGroupActive = computed(() => {
  const p = route.path;
  return (
    p === "/knowledge" ||
    p.startsWith("/knowledge/") ||
    p === "/memories" ||
    p.startsWith("/memories/") ||
    p === "/agents" ||
    p.startsWith("/agents/") ||
    p === "/credentials"
  );
});

function isAgentChatPath(path: string): boolean {
  return /^\/agents\/[^/]+\/chat/.test(path);
}

function isActive(to: string) {
  return isActiveRoute(route.path, to);
}

const moreNavActive = computed(() => mobileMorePaths.value.has(route.path));

const pageTitle = computed(() => pageTitleForPath(route.path) || "Alga");

const headerBrandIconSrc = computed(() => {
  const brand = pageHeader.value?.headerAgentBrand;
  return brand ? getAgentBrandIconSrc(brand) : null;
});

const showBackButton = computed(() => {
  return (
    route.path.startsWith("/alerts/") ||
    route.path.startsWith("/incidents/") ||
    (route.path.startsWith("/agents/") && route.path.endsWith("/chat")) ||
    route.path.startsWith("/teams/") ||
    route.path.startsWith("/services/") ||
    route.path.startsWith("/on-call/schedules/") ||
    route.path.endsWith("/post-mortem")
  );
});

const hideMobileChatChrome = computed(() => {
  return isAgentChatPath(route.path);
});

watch(
  () => auth.user,
  (user) => {
    if (user) {
      notificationStore.fetchUnreadCount();
      notificationSSE.reconnect();
    } else {
      notificationStore.reset();
      notificationSSE.close();
      closeGlobalSearch();
    }
  },
  { immediate: true },
);

// Cmd-K is a global shortcut, but the overlay only renders for authenticated
// users, so the handler must no-op on public routes to avoid opening the
// overlay into a detached state.
function onGlobalSearchKeydown(e: KeyboardEvent) {
  if (!auth.user) return;
  handleGlobalSearchKeydown(e);
}

onMounted(() => {
  document.addEventListener("keydown", onGlobalSearchKeydown);
});

onBeforeUnmount(() => {
  document.removeEventListener("keydown", onGlobalSearchKeydown);
});
</script>

<template>
  <div
    v-if="route.meta.public || route.path === '/onboarding'"
    class="bg-[var(--bg-primary)] text-[var(--text-primary)]"
  >
    <RouterView />
    <ToastStack />
  </div>
  <div
    v-else-if="auth.user"
    class="flex h-screen overflow-hidden bg-[var(--bg-primary)] text-[var(--text-primary)]"
  >
    <!-- Desktop sidebar -->
    <div class="hidden shrink-0 md:block">
      <Sidebar />
    </div>

    <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <header
        class="flex h-14 min-w-0 shrink-0 items-center justify-between gap-2 border-b border-[var(--border-primary)] bg-[var(--header-bg)] px-4 md:px-6"
      >
        <ChatSearchBar
          v-if="headerSearchActive"
          :query="headerSearchState.query"
          :match-count="headerSearchState.matchCount"
          :current-index="headerSearchState.currentIndex"
          class="flex-1 min-w-0"
          @update:query="headerSearchState.onUpdateQuery"
          @next="headerSearchState.onNext"
          @prev="headerSearchState.onPrev"
          @close="headerSearchState.onClose"
        />
        <template v-else>
          <div
            class="hidden min-w-0 flex-1 overflow-hidden text-lg font-semibold md:flex md:items-center md:gap-2"
          >
            <button
              v-if="showBackButton"
              type="button"
              aria-label="Go back"
              class="flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
              @click="router.back()"
            >
              <ChevronLeft class="h-5 w-5" aria-hidden="true" />
            </button>
            <template v-if="pageHeader">
              <span
                v-for="(badge, i) in pageHeader.leadingBadges ?? []"
                :key="`l-${i}`"
                class="shrink-0 px-2 py-0.5 rounded-full text-xs font-medium"
                :class="badge.cssClass"
                >{{ badge.text }}</span
              >
              <span
                v-if="pageHeader.titlePrefix"
                class="shrink-0 font-mono text-sm text-[var(--text-muted)]"
                >{{ pageHeader.titlePrefix }}</span
              >
              <img
                v-if="headerBrandIconSrc"
                :src="headerBrandIconSrc"
                alt=""
                width="20"
                height="20"
                class="h-5 w-5 shrink-0 rounded-full object-cover"
              />
              <Bot
                v-else-if="pageHeader.headerAgentBrand === 'other'"
                class="h-5 w-5 shrink-0 text-[var(--text-muted)]"
                aria-hidden="true"
              />
              <component v-if="pageHeader.titleIcon" :is="pageHeader.titleIcon" />
              <span class="min-w-0 truncate">{{ pageHeader.title }}</span>
              <span
                v-for="(badge, i) in pageHeader.badges ?? []"
                :key="`b-${i}`"
                class="shrink-0 px-2 py-0.5 rounded-full text-xs font-medium"
                :class="badge.cssClass"
                >{{ badge.text }}</span
              >
            </template>
            <template v-else>{{ pageTitle }}</template>
          </div>
          <div class="flex min-w-0 flex-1 items-center gap-2 overflow-hidden md:hidden">
            <button
              v-if="showBackButton"
              type="button"
              aria-label="Go back"
              :class="CARD_ICON_BTN_CLASS"
              @click="router.back()"
            >
              <ChevronLeft class="h-5 w-5" aria-hidden="true" />
            </button>
            <template v-if="pageHeader">
              <span
                v-for="(badge, i) in pageHeader.leadingBadges ?? []"
                :key="`ml-${i}`"
                class="shrink-0 px-2 py-0.5 rounded-full text-xs font-medium"
                :class="badge.cssClass"
                >{{ badge.text }}</span
              >
              <span
                v-if="pageHeader.titlePrefix"
                class="shrink-0 font-mono text-sm text-[var(--text-muted)]"
                >{{ pageHeader.titlePrefix }}</span
              >
              <img
                v-if="headerBrandIconSrc"
                :src="headerBrandIconSrc"
                alt=""
                width="20"
                height="20"
                class="h-5 w-5 shrink-0 rounded-full object-cover"
              />
              <Bot
                v-else-if="pageHeader.headerAgentBrand === 'other'"
                class="h-5 w-5 shrink-0 text-[var(--text-muted)]"
                aria-hidden="true"
              />
              <component v-if="pageHeader.titleIcon" :is="pageHeader.titleIcon" />
              <span class="min-w-0 truncate text-lg font-semibold">{{ pageHeader.title }}</span>
              <span
                v-for="(badge, i) in pageHeader.badges ?? []"
                :key="`mb-${i}`"
                class="shrink-0 px-2 py-0.5 rounded-full text-xs font-medium"
                :class="badge.cssClass"
                >{{ badge.text }}</span
              >
            </template>
            <span v-else class="truncate text-lg font-semibold">{{ pageTitle }}</span>
          </div>
          <div class="flex min-w-0 items-center gap-2">
            <component
              v-for="(action, i) in pageHeader?.actions ?? []"
              :key="`a-${i}`"
              :is="action"
            />
          </div>
        </template>
      </header>

      <main
        class="min-h-0 flex-1 overflow-y-auto"
        :class="[hideMobileChatChrome ? '' : 'pb-24 md:pb-0']"
      >
        <RouterView v-slot="{ Component }">
          <KeepAlive>
            <Suspense>
              <component :is="Component" />
              <template #fallback>
                <div class="flex min-h-[60vh] items-center justify-center">
                  <LoadingSpinner centered label="Loading page..." />
                </div>
              </template>
            </Suspense>
          </KeepAlive>
        </RouterView>
      </main>
    </div>

    <!-- Mobile bottom navigation -->
    <nav
      v-if="!hideMobileChatChrome"
      class="fixed bottom-0 left-0 right-0 z-[21] flex h-16 border-t border-[var(--border-primary)] bg-[var(--bg-sidebar)] pb-[env(safe-area-inset-bottom)] md:hidden"
      aria-label="Mobile navigation"
    >
      <router-link
        v-for="item in navItems"
        :key="item.to"
        :to="item.to"
        class="mobile-bottom-nav-item"
        :class="isActive(item.to) ? 'active' : ''"
        @mouseenter="prefetch(item.to)"
        @focus="prefetch(item.to)"
      >
        <div v-if="isActive(item.to)" class="active-indicator" />
        <component :is="item.icon" class="h-5 w-5" />
        <span>{{ item.label }}</span>
      </router-link>
      <button
        class="mobile-bottom-nav-item"
        :class="agentGroupActive || agentMenuOpen ? 'active' : ''"
        @click="agentMenuOpen = !agentMenuOpen"
      >
        <div v-if="agentGroupActive || agentMenuOpen" class="active-indicator" />
        <Bot class="h-5 w-5" />
        <span>Agents</span>
      </button>
      <button
        class="mobile-bottom-nav-item"
        :class="moreNavActive || moreMenuOpen ? 'active' : ''"
        @click="moreMenuOpen = !moreMenuOpen"
      >
        <div v-if="moreNavActive || moreMenuOpen" class="active-indicator" />
        <Menu class="h-5 w-5" />
        <span>More</span>
      </button>
    </nav>
    <MobileAgentMenu v-if="agentMenuOpen && !hideMobileChatChrome" @close="agentMenuOpen = false" />
    <MobileMoreMenu
      v-if="moreMenuOpen && !hideMobileChatChrome"
      @close="moreMenuOpen = false"
      @open-settings="openSettings()"
    />
    <SettingsDialog :open="showSettings" :tab="settingsTab" @close="closeSettings" />
    <GlobalSearchOverlay />
    <RouteLoadingBar />
    <ToastStack />
  </div>
  <div
    v-else
    class="flex h-screen items-center justify-center bg-[var(--bg-primary)] text-[var(--text-primary)]"
    role="status"
    aria-live="polite"
    aria-busy="true"
  >
    <Loader2 class="h-8 w-8 shrink-0 animate-spin text-[var(--text-muted)]" aria-hidden="true" />
    <span class="sr-only">Loading session</span>
    <ToastStack />
  </div>
</template>
