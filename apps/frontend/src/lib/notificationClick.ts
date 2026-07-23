import type { Router } from "vue-router";
import type { NotificationRecord } from "@/lib/api";
import type { useNotificationStore } from "@/stores/notifications";

const SAFE_ID = /^[A-Za-z0-9_-]{1,128}$/;

export async function handleNotificationClick(
  notification: NotificationRecord,
  store: ReturnType<typeof useNotificationStore>,
  router: Router,
  onClose?: () => void,
): Promise<void> {
  if (!notification.read) {
    await store.markRead(notification.id);
  }
  onClose?.();
  const routes: Record<string, string> = {
    alert: "/alerts",
    incident: "/incidents",
    service: "/services",
    knowledge: "/knowledge",
    schedule: "/on-call",
  };
  const basePath = routes[notification.resource_type];
  if (basePath && notification.resource_id && SAFE_ID.test(notification.resource_id)) {
    router.push(`${basePath}/${notification.resource_id}`);
  }
}
