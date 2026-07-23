import { ref, computed } from "vue";
import { defineStore } from "pinia";
import { api, type NotificationRecord } from "@/lib/api";
import { MAX_NOTIFICATIONS } from "@/lib/threadLimits";

export const useNotificationStore = defineStore("notifications", () => {
  const notifications = ref<NotificationRecord[]>([]);
  const unreadCount = ref(0);
  const loading = ref(false);
  const seenIds = computed(() => new Set(notifications.value.map((n) => n.id)));

  const hasUnread = computed(() => unreadCount.value > 0);

  async function fetchUnreadCount() {
    try {
      const res = await api.getUnreadNotificationCount();
      unreadCount.value = res.count ?? 0;
    } catch {
      // silently ignore
    }
  }

  async function fetchNotifications(limit = 20, skip = 0) {
    loading.value = true;
    try {
      const res = await api.getNotifications(limit, skip);
      const batch = res ?? [];
      if (skip > 0) {
        const seen = new Set(notifications.value.map((n) => n.id));
        for (const n of batch) {
          if (!seen.has(n.id)) {
            notifications.value.push(n);
            seen.add(n.id);
          }
        }
        notifications.value = notifications.value.slice(0, MAX_NOTIFICATIONS);
      } else {
        notifications.value = batch.slice(0, MAX_NOTIFICATIONS);
      }
    } catch {
      // silently ignore
    } finally {
      loading.value = false;
    }
  }

  let markAllInFlight = false;

  async function markRead(id: string) {
    // During markAllRead we skip local mutation entirely and let the
    // server-driven notification_unread_count SSE reconcile the UI. Otherwise
    // an optimistic decrement plus a failed inner markRead would leave the
    // local state marked-read but the server would still report it unread,
    // causing the badge to drift from the server's truth.
    const idx = notifications.value.findIndex((n) => n.id === id);
    const hadUnread = idx !== -1 && !notifications.value[idx].read;
    if (!markAllInFlight && hadUnread) {
      notifications.value[idx] = { ...notifications.value[idx], read: true };
      unreadCount.value = Math.max(0, unreadCount.value - 1);
    }
    try {
      await api.markNotificationRead(id);
    } catch {
      if (!markAllInFlight && idx !== -1 && hadUnread) {
        notifications.value[idx] = { ...notifications.value[idx], read: false };
        unreadCount.value += 1;
      }
    }
  }

  async function markAllRead() {
    markAllInFlight = true;
    const prevCount = unreadCount.value;
    const prevNotifications = [...notifications.value];
    notifications.value = notifications.value.map((n) => ({ ...n, read: true }));
    unreadCount.value = 0;
    try {
      await api.markAllNotificationsRead();
    } catch {
      unreadCount.value = prevCount;
      notifications.value = prevNotifications;
    } finally {
      markAllInFlight = false;
    }
  }

  function handleSSEEvent(eventType: string, data: unknown) {
    if (eventType === "notification_new") {
      const n = data as NotificationRecord;
      if (!n?.id || typeof n.id !== "string") return;
      if (seenIds.value.has(n.id)) return;
      notifications.value = [n, ...notifications.value].slice(0, MAX_NOTIFICATIONS);
      unreadCount.value += 1;
    } else if (eventType === "notification_unread_count") {
      const d = data as { count: number };
      if (typeof d?.count === "number") {
        unreadCount.value = d.count;
      }
    }
  }

  function reset() {
    notifications.value = [];
    unreadCount.value = 0;
    loading.value = false;
  }

  return {
    notifications,
    unreadCount,
    loading,
    hasUnread,
    fetchUnreadCount,
    fetchNotifications,
    markRead,
    markAllRead,
    handleSSEEvent,
    reset,
  };
});
