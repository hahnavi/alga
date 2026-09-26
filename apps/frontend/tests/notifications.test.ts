import { beforeEach, describe, expect, it } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { useNotificationStore } from "../src/stores/notifications.ts";

// The backend publishes exactly two notification SSE event types:
// `notification` (dispatch worker, born-unread, user-scoped payload without
// `user_id`/`read`) and `notification_unread_count` (read endpoints). The
// store deliberately handles only these two — see spec discrepancy D2.
const dispatchPayload = {
  id: "n1",
  type: "escalation",
  title: "Paging",
  message: "Escalation level 1",
  resource_type: "incident",
  resource_id: "42",
  created_at: "2026-08-26T10:00:00Z",
};

describe("notifications store handleSSEEvent", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("normalizes a dispatch-worker `notification` event into a born-unread record", () => {
    const store = useNotificationStore();
    store.handleSSEEvent("notification", dispatchPayload);

    expect(store.notifications).toHaveLength(1);
    const n = store.notifications[0];
    expect(n.id).toBe("n1");
    expect(n.read).toBe(false);
    expect(n.user_id).toBe("");
    expect(n.resource_id).toBe("42");
    expect(store.unreadCount).toBe(1);
  });

  it("reconciles the badge from `notification_unread_count` events", () => {
    const store = useNotificationStore();
    store.handleSSEEvent("notification", dispatchPayload);
    store.handleSSEEvent("notification_unread_count", { count: 7 });

    expect(store.unreadCount).toBe(7);

    // Malformed count events keep the existing value.
    store.handleSSEEvent("notification_unread_count", null);
    store.handleSSEEvent("notification_unread_count", { count: -1 });
    expect(store.unreadCount).toBe(7);
  });

  it("drops malformed events instead of corrupting state", () => {
    const store = useNotificationStore();
    store.handleSSEEvent("notification", { id: "x" }); // missing required fields
    store.handleSSEEvent("notification", null);

    expect(store.notifications).toHaveLength(0);
    expect(store.unreadCount).toBe(0);
  });

  it("dedupes by id and counts unread separately from dedupe", () => {
    const store = useNotificationStore();
    store.handleSSEEvent("notification", dispatchPayload);
    store.handleSSEEvent("notification", { ...dispatchPayload });
    expect(store.notifications).toHaveLength(1);
    expect(store.unreadCount).toBe(1);

    store.handleSSEEvent("notification", { ...dispatchPayload, id: "n2" });
    expect(store.notifications).toHaveLength(2);
    expect(store.unreadCount).toBe(2);
  });
});
