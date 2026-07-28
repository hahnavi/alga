import { expect, test } from "@playwright/test";
import { dataEnvelope, json, makeNotification, mockAuthenticated } from "./helpers";

const UNREAD = makeNotification();
const READ = makeNotification({
  id: "notif-002",
  type: "mention",
  title: "Mention in incident thread",
  message: "@admin can you take a look?",
  read: true,
  resource_type: "incident",
  resource_id: "7",
});

test.describe("notifications: inbox", () => {
  test("renders all notifications with unread indicator", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/notifications**", (route) =>
      route.fulfill(dataEnvelope([UNREAD, READ])),
    );

    await page.goto("/notifications");
    await expect(page.getByText("Incident #5 declared")).toBeVisible();
    await expect(page.getByText("Mention in incident thread")).toBeVisible();
    await expect(page.getByRole("button", { name: /unread/i })).toBeVisible();
  });

  test("unread filter hides read notifications", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/notifications**", (route) =>
      route.fulfill(dataEnvelope([UNREAD, READ])),
    );

    await page.goto("/notifications");
    await expect(page.getByText("Mention in incident thread")).toBeVisible();
    await page.getByRole("button", { name: /unread/i }).click();
    await expect(page.getByText("Mention in incident thread")).toBeHidden();
    await expect(page.getByText("Incident #5 declared")).toBeVisible();
  });

  test("shows empty state when there are no notifications", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));

    await page.goto("/notifications");
    await expect(page.getByText(/no notifications yet/i)).toBeVisible();
  });

  test("empty unread filter has its own empty state", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/notifications**", (route) =>
      route.fulfill(dataEnvelope([{ ...READ }])),
    );

    await page.goto("/notifications");
    await page.getByRole("button", { name: /unread/i }).click();
    await expect(page.getByText(/no unread notifications/i)).toBeVisible();
  });
});

test.describe("notifications: actions", () => {
  test("mark all read posts to read-all", async ({ page }) => {
    await mockAuthenticated(page);

    let markAllCalled = false;
    await page.route("**/api/v1/notifications**", (route) => {
      if (route.request().url().includes("/unread-count")) {
        return route.fulfill(json({ count: 1 }));
      }
      if (route.request().url().includes("/read-all")) {
        markAllCalled = true;
        return route.fulfill(json({ status: "ok" }));
      }
      return route.fulfill(dataEnvelope([UNREAD, READ]));
    });

    await page.goto("/notifications");
    await page.getByRole("button", { name: /mark all read/i }).click();
    await expect.poll(() => markAllCalled).toBe(true);
  });

  test("clicking an unread incident notification marks it read and navigates", async ({ page }) => {
    await mockAuthenticated(page);

    let markReadCalled = false;
    await page.route("**/api/v1/notifications**", (route) => {
      if (route.request().url().includes("/unread-count")) {
        return route.fulfill(json({ count: 1 }));
      }
      if (route.request().url().includes("/notif-001/read")) {
        markReadCalled = true;
        return route.fulfill(json({ status: "ok" }));
      }
      return route.fulfill(dataEnvelope([UNREAD, READ]));
    });

    await page.goto("/notifications");
    await page.getByText("Incident #5 declared").click();

    await expect.poll(() => markReadCalled).toBe(true);
    await expect(page).toHaveURL(/\/incidents\/5/);
  });
});
