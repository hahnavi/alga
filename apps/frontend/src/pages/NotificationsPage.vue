<script setup lang="ts">
import { ref, computed, h, onMounted, watch } from "vue";
import { useRouter } from "vue-router";
import { Bell, CheckCheck, Settings, Users, Inbox } from "@lucide/vue";
import { useNotificationStore } from "@/stores/notifications";
import { useAuthStore } from "@/stores/auth";
import type { NotificationRecord } from "@/lib/api";
import { formatTimeAgo, formatTime } from "@/lib/time";
import { handleNotificationClick } from "@/lib/notificationClick";
import { usePageHeader } from "@/composables/usePageHeader";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";
import Button from "@/components/ui/Button.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";

defineOptions({ name: "NotificationsPage" });

const auth = useAuthStore();
const store = useNotificationStore();
const router = useRouter();
const filter = ref<"all" | "unread">("all");
const loadingMore = ref(false);
const allLoaded = ref(false);

const filtered = computed(() => {
  if (filter.value === "unread") return store.notifications.filter((n) => !n.read);
  return store.notifications;
});

async function clickNotification(n: NotificationRecord) {
  await handleNotificationClick(n, store, router);
}

async function markAllRead() {
  await store.markAllRead();
}

async function loadMore() {
  if (loadingMore.value || allLoaded.value) return;
  loadingMore.value = true;
  try {
    const before = store.notifications.length;
    await store.fetchNotifications(20, before);
    if (store.notifications.length === before) {
      allLoaded.value = true;
    }
  } finally {
    loadingMore.value = false;
  }
}

async function refresh() {
  allLoaded.value = false;
  await store.fetchNotifications(20, 0);
}

usePageHeader(() => ({
  title: "Notifications",
  options: {
    titleIcon: h(Bell, {
      class: "h-5 w-5 shrink-0 text-[var(--text-muted)]",
      "aria-hidden": "true",
    }),
    actions: [
      h(
        "button",
        {
          type: "button",
          class: HEADER_ICON_BTN_CLASS,
          "aria-label": "Notification Preferences",
          title: "Notification Preferences",
          onClick: () => router.push("/settings/notifications"),
        },
        [h(Settings, { class: "h-4 w-4", "aria-hidden": "true" })],
      ),
    ],
  },
}));

onMounted(() => {
  refresh();
});

watch(
  () => auth.user,
  (u) => {
    if (u) refresh();
  },
);
</script>

<template>
  <div class="mx-auto max-w-2xl px-4 py-4 md:px-6 md:py-6">
    <div
      class="mb-4 flex gap-1 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-1"
    >
      <button
        type="button"
        class="flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors cursor-pointer"
        :class="
          filter === 'all'
            ? 'bg-[var(--bg-card)] text-[var(--text-primary)] shadow-sm'
            : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'
        "
        @click="filter = 'all'"
      >
        All
      </button>
      <button
        type="button"
        class="flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors cursor-pointer"
        :class="
          filter === 'unread'
            ? 'bg-[var(--bg-card)] text-[var(--text-primary)] shadow-sm'
            : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'
        "
        @click="filter = 'unread'"
      >
        Unread{{ store.hasUnread ? ` (${store.unreadCount})` : "" }}
      </button>
    </div>

    <div v-if="store.hasUnread" class="mb-4 flex justify-end">
      <Button size="sm" @click="markAllRead">
        <CheckCheck class="h-3.5 w-3.5" />
        Mark all read
      </Button>
    </div>

    <LoadingSpinner
      v-if="store.loading && store.notifications.length === 0"
      centered
      label="Loading notifications..."
    />

    <EmptyState
      v-else-if="filtered.length === 0"
      :message="filter === 'unread' ? 'No unread notifications' : 'No notifications yet'"
    >
      <template #icon>
        <Inbox class="mb-2 h-6 w-6 opacity-40" />
      </template>
    </EmptyState>

    <div v-else class="space-y-1">
      <button
        v-for="n in filtered"
        :key="n.id"
        type="button"
        class="flex w-full cursor-pointer gap-3 rounded-lg border border-[var(--border-primary)] px-4 py-3 text-left transition-colors hover:bg-[var(--sidebar-hover,rgb(148_163_184/0.08))]"
        :class="
          n.read ? 'bg-[var(--bg-card)]' : 'bg-[var(--bg-card)] border-l-2 border-l-[var(--accent)]'
        "
        @click="clickNotification(n)"
      >
        <div class="mt-0.5 shrink-0">
          <div
            class="flex h-8 w-8 items-center justify-center rounded-full"
            :class="
              n.type === 'mention'
                ? 'bg-[var(--bg-badge-info)] text-[var(--text-badge-info)]'
                : 'bg-[var(--bg-primary)] text-[var(--text-muted)]'
            "
          >
            <Users v-if="n.type === 'mention'" class="h-4 w-4" />
            <Bell v-else class="h-4 w-4" />
          </div>
        </div>
        <div class="min-w-0 flex-1">
          <p
            class="text-sm text-[var(--text-primary)]"
            :class="n.read ? 'font-normal' : 'font-medium'"
          >
            {{ n.title }}
          </p>
          <p v-if="n.message" class="mt-0.5 text-xs text-[var(--text-muted)]">{{ n.message }}</p>
          <p class="mt-1 text-[10px] text-[var(--text-muted)]" :title="formatTime(n.created_at)">
            {{ formatTimeAgo(n.created_at) }}
          </p>
        </div>
        <div v-if="!n.read" class="mt-3 shrink-0">
          <span class="inline-block h-2 w-2 rounded-full bg-[var(--accent)]" />
        </div>
      </button>

      <div v-if="!allLoaded" class="pt-2 text-center">
        <Button size="sm" :disabled="loadingMore" @click="loadMore">
          {{ loadingMore ? "Loading..." : "Load more" }}
        </Button>
      </div>
    </div>
  </div>
</template>
