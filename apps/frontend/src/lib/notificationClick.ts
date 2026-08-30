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
    // Post-mortem-scoped notifications: there is no per-action-item or
    // per-post-mortem route, so land on the post-mortems list.
    post_mortem: "/post-mortems",
    action_item: "/post-mortems",
  };
  const basePath = routes[notification.resource_type];
  if (!basePath) return;
  // Types whose resource_id is not a routable path segment (action items and
  // post-mortems have no detail route keyed by their id) go to the bare path.
  if (
    notification.resource_type === "post_mortem" ||
    notification.resource_type === "action_item"
  ) {
    router.push(basePath);
    return;
  }
  if (notification.resource_id && SAFE_ID.test(notification.resource_id)) {
    router.push(`${basePath}/${notification.resource_id}`);
  }
}
