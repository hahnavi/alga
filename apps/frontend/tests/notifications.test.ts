import { beforeEach, describe, expect, it } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { useNotificationStore } from "../src/stores/notifications.ts";
import type { NotificationRecord } from "../src/lib/api.ts";

const dispatchPayload = {
  id: "n1",
  type: "escalation",
  title: "Paging",
  message: "Escalation level 1",
  resource_type: "incident",
  resource_id: "42",
  created_at: "2026-08-26T10:00:00Z",
};

const fullRecord = {
  id: "n2",
  user_id: "user-1",
  type: "test",
  title: "Test notification",
  message: "hello",
  read: false,
  resource_type: "system",
  resource_id: "",
  created_at: "2026-08-26T10:01:00Z",
} satisfies NotificationRecord;

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
    expect(n.resource_id).toBe("42");
    expect(store.unreadCount).toBe(1);
  });

  it("inserts a full record from the API-side `notification_new` event", () => {
    const store = useNotificationStore();
    store.handleSSEEvent("notification_new", { ...fullRecord });

    expect(store.notifications).toHaveLength(1);
    expect(store.notifications[0].id).toBe("n2");
    expect(store.unreadCount).toBe(1);
  });

  it("drops malformed events instead of corrupting state", () => {
    const store = useNotificationStore();
    store.handleSSEEvent("notification", { id: "x" }); // missing required fields
    store.handleSSEEvent("notification_new", null);

    expect(store.notifications).toHaveLength(0);
    expect(store.unreadCount).toBe(0);
  });

  it("dedupes by id across event types and counts unread separately from dedupe", () => {
    const store = useNotificationStore();
    store.handleSSEEvent("notification", dispatchPayload);
    store.handleSSEEvent("notification", { ...dispatchPayload });
    // Same id arriving via the other path must not double-insert either.
    store.handleSSEEvent("notification_new", {
      ...fullRecord,
      id: "n1",
    });

    expect(store.notifications).toHaveLength(1);
    expect(store.unreadCount).toBe(1);
  });
});
