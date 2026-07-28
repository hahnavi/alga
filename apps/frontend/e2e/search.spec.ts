import { expect, test, type Page } from "@playwright/test";
import {
  dataEnvelope,
  makeAlert,
  makeIncident,
  mockAuthenticated,
  mockDashboardApis,
} from "./helpers";

test.describe("global search (Cmd/Ctrl+K)", () => {
  async function gotoDashboardReady(page: Page) {
    await page.goto("/");
    // The Cmd/Ctrl+K listener is registered when App.vue mounts, so wait for
    // the shell chrome before sending the shortcut.
    await expect(page.getByRole("link", { name: "Dashboard" })).toBeVisible();
  }

  test("opens with keyboard shortcut and closes with Escape", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page);

    await gotoDashboardReady(page);
    await expect(page.locator("[data-global-search-input]")).toBeHidden();

    await page.keyboard.press("Control+k");
    await expect(page.locator("[data-global-search-input]")).toBeVisible();

    await page.keyboard.press("Escape");
    await expect(page.locator("[data-global-search-input]")).toBeHidden();
  });

  test("searches alerts and incidents and navigates to the clicked alert", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page);

    await page.route("**/api/v1/alerts**", (route) =>
      route.fulfill(dataEnvelope([makeAlert({ alert_number: 1 })])),
    );
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [makeIncident({ incident_number: 9, title: "Redis outage" })],
          total: 1,
        }),
      ),
    );
    await page.route("**/api/v1/alerts/1**", (route) =>
      route.fulfill(dataEnvelope({ alert: makeAlert({ alert_number: 1 }) })),
    );
    await page.route("**/api/v1/alerts/1/related**", (route) =>
      route.fulfill(dataEnvelope({ related_alerts: [], incident: null })),
    );

    await gotoDashboardReady(page);
    await page.keyboard.press("Control+k");
    const input = page.locator("[data-global-search-input]");
    await expect(input).toBeVisible();

    await input.fill("cpu");
    await input.press("Enter");

    await expect(page.getByText("HighCPU").first()).toBeVisible();

    await page.getByRole("tab", { name: /incidents/i }).click();
    await expect(page.getByText("Redis outage").first()).toBeVisible();

    await page.getByRole("tab", { name: /alerts/i }).click();
    await page.getByText("HighCPU").first().click();
    await expect(page).toHaveURL(/\/alerts\/1/);
  });

  test("incident tab lists incident matches", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page);

    await page.route("**/api/v1/alerts**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [makeIncident({ incident_number: 9, title: "Redis outage" })],
          total: 1,
        }),
      ),
    );

    await gotoDashboardReady(page);
    await page.keyboard.press("Control+k");
    const input = page.locator("[data-global-search-input]");
    await input.fill("redis");
    await input.press("Enter");

    // With zero alert matches the overlay auto-switches to the incidents tab.
    await expect(page.getByText("Redis outage").first()).toBeVisible();
    await page.getByRole("tab", { name: /incidents/i }).click();
    await expect(page.getByText("Redis outage").first()).toBeVisible();
  });
});

test.describe("alerts page inline search", () => {
  test("header search issues a search query to the alerts API", async ({ page }) => {
    await mockAuthenticated(page);

    const alertUrls: string[] = [];
    await page.route("**/api/v1/alerts**", (route) => {
      alertUrls.push(route.request().url());
      return route.fulfill(dataEnvelope([]));
    });

    await page.goto("/alerts");
    await page.getByRole("button", { name: "Search" }).click();
    await page.locator("[data-page-header-search]").fill("memory");

    await expect
      .poll(() => alertUrls.some((u) => new URL(u).searchParams.get("search") === "memory"))
      .toBe(true);
  });
});
