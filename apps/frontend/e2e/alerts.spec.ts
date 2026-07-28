import { expect, test } from "@playwright/test";
import { dataEnvelope, makeAlert, mockAuthenticated } from "./helpers";

test.describe("alerts: list", () => {
  test("renders alert rows from API response", async ({ page }) => {
    await mockAuthenticated(page);
    const alerts = [
      makeAlert({ alert_number: 1, labels: { alertname: "HighCPU", severity: "critical" } }),
      makeAlert({
        alert_number: 2,
        fingerprint: "fp-002",
        status: "resolved",
        labels: { alertname: "DiskFull", severity: "warning" },
      }),
    ];
    await page.route("**/api/v1/alerts**", (route) => route.fulfill(dataEnvelope(alerts)));

    await page.goto("/alerts");
    await expect(page.getByText("HighCPU")).toBeVisible();
    await expect(page.getByText("DiskFull")).toBeVisible();
  });

  test("shows empty state when no alerts", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/alerts**", (route) => route.fulfill(dataEnvelope([])));

    await page.goto("/alerts");
    await expect(page.getByText(/no alerts|empty/i)).toBeVisible();
  });

  test("displays firing and resolved status indicators", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/alerts**", (route) =>
      route.fulfill(
        dataEnvelope([
          makeAlert({ alert_number: 1, status: "firing" }),
          makeAlert({ alert_number: 2, fingerprint: "fp-002", status: "resolved" }),
        ]),
      ),
    );

    await page.goto("/alerts");
    await expect(page.getByText(/firing/i).first()).toBeVisible();
    await expect(page.getByText(/resolved/i).first()).toBeVisible();
  });
});

test.describe("alerts: detail", () => {
  test("shows alert detail with labels and annotations", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({
      alert_number: 42,
      labels: { alertname: "MemoryLeak", severity: "critical", service: "api" },
      annotations: { summary: "Memory usage growing unbounded" },
    });
    await page.route("**/api/v1/alerts/42**", (route) => {
      if (route.request().url().includes("/related")) {
        return route.fulfill(dataEnvelope({ related_alerts: [], incident: null }));
      }
      return route.fulfill(dataEnvelope({ alert }));
    });

    await page.goto("/alerts/42");
    await expect(page.getByText("MemoryLeak").first()).toBeVisible();
    await expect(page.getByText(/memory usage growing unbounded/i)).toBeVisible();
  });

  test("acknowledge button sends POST and updates state", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({ alert_number: 7, acknowledged: false });
    await page.route("**/api/v1/alerts/7", (route) => route.fulfill(dataEnvelope({ alert })));
    await page.route("**/api/v1/alerts/7/related", (route) =>
      route.fulfill(dataEnvelope({ related_alerts: [], incident: null })),
    );

    let ackCalled = false;
    await page.route("**/api/v1/alerts/7/acknowledge", (route) => {
      ackCalled = true;
      return route.fulfill(dataEnvelope({ ...alert, acknowledged: true }));
    });

    await page.goto("/alerts/7");
    await page.getByRole("button", { name: /acknowledge/i }).click();
    await expect.poll(() => ackCalled).toBe(true);
  });

  test("resolve action sends POST via actions menu", async ({ page }) => {
    await mockAuthenticated(page);
    // Acknowledged so the header renders the workflow actions menu with
    // "Mark resolved" instead of the inline Acknowledge button.
    const alert = makeAlert({ alert_number: 8, status: "firing", acknowledged: true });
    await page.route("**/api/v1/alerts/8", (route) => route.fulfill(dataEnvelope({ alert })));
    await page.route("**/api/v1/alerts/8/related", (route) =>
      route.fulfill(dataEnvelope({ related_alerts: [], incident: null })),
    );

    let resolveCalled = false;
    await page.route("**/api/v1/alerts/8/resolve", (route) => {
      resolveCalled = true;
      return route.fulfill(dataEnvelope({ ...alert, status: "resolved" }));
    });

    await page.goto("/alerts/8");
    await page.getByRole("button", { name: "Alert actions" }).click();
    await page.getByRole("menuitem", { name: "Mark resolved" }).click();
    await expect.poll(() => resolveCalled).toBe(true);
  });
});
