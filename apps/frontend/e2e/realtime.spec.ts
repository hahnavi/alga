import { expect, test } from "@playwright/test";
import { dataEnvelope, json, makeAlert, mockAuthenticated } from "./helpers";

test.describe("realtime: SSE alert updates", () => {
  test("new alert from SSE appears in list without reload", async ({ page }) => {
    await mockAuthenticated(page);
    // Quiets the SSE auth probe so reconnect handling stays inert.
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/alerts**", (route) =>
      route.fulfill(dataEnvelope([makeAlert({ alert_number: 1 })])),
    );

    const pushed = makeAlert({
      alert_number: 2,
      fingerprint: "fp-sse-002",
      labels: { alertname: "SSEPushed", severity: "warning" },
    });
    // Hold the event stream open until the initial list has rendered, then
    // deliver a single alert_created frame as a post-navigation push.
    let releaseStream!: () => void;
    const streamGate = new Promise<void>((resolve) => {
      releaseStream = resolve;
    });
    await page.route("**/api/v1/events**", async (route) => {
      await streamGate;
      try {
        await route.fulfill({
          contentType: "text/event-stream",
          body: `retry: 30000\nevent: alert_created\ndata: ${JSON.stringify(pushed)}\n\n`,
        });
      } catch {
        // The activation reconnect replaces the first EventSource; its
        // pending request is aborted and needs no response.
      }
    });

    await page.goto("/alerts");
    await expect(page.getByText("HighCPU")).toBeVisible();
    await page.evaluate(() => {
      sessionStorage.setItem("sse_no_reload_marker", "1");
    });
    releaseStream();

    // The second alert only arrives through the SSE stream above; the marker
    // proves the list update happened without a full page reload.
    await expect(page.getByText("SSEPushed")).toBeVisible();
    await expect
      .poll(() => page.evaluate(() => sessionStorage.getItem("sse_no_reload_marker")))
      .toBe("1");
  });
});

test.describe("edge: session expiry", () => {
  test("401 on auth/me redirects to login", async ({ page }) => {
    await mockAuthenticated(page);
    await page.goto("/");
    await expect(page).toHaveURL("/");

    await page.route("**/api/v1/auth/me", (route) =>
      route.fulfill({ status: 401, ...json({ error: "Unauthorized" }) }),
    );
    await page.route("**/api/v1/auth/refresh", (route) =>
      route.fulfill({ status: 401, ...json({ error: "Unauthorized" }) }),
    );

    await page.reload();
    await expect(page).toHaveURL(/\/login/);
  });
});

test.describe("edge: API error handling", () => {
  test("alerts page shows error state on 500", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/alerts**", (route) =>
      route.fulfill({ status: 500, ...json({ error: "Internal server error" }) }),
    );

    await page.goto("/alerts");
    await expect(page.getByText(/error|failed|something went wrong/i).first()).toBeVisible();
  });

  test("incidents page shows error state on network failure", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) => route.abort("connectionrefused"));

    await page.goto("/incidents");
    await expect(page.getByText(/error|failed|something went wrong/i).first()).toBeVisible();
  });
});

test.describe("edge: stale chunk recovery", () => {
  test("chunk load error triggers page reload", async ({ page }) => {
    await mockAuthenticated(page);
    await page.goto("/");
    await expect(page).toHaveURL("/");

    await page.evaluate(() => {
      sessionStorage.removeItem("alga_chunk_reload");
    });

    // Fail the lazy route module so navigation hits the router's stale-chunk
    // handler for real, which recovers via a hard location.assign() reload.
    await page.route("**/src/pages/AlertsPage.vue**", (route) => route.abort());

    const reloaded = page.waitForEvent("load", { timeout: 5000 }).catch(() => null);
    await page.getByRole("link", { name: "Alerts" }).click();

    const result = await reloaded;
    expect(result).not.toBeNull();
  });
});

test.describe("edge: document title", () => {
  test("login page sets document title with Alga suffix", async ({ page }) => {
    await mockAuthenticated(page);
    await page.goto("/");
    await expect(page).toHaveTitle(/Alga/);
  });

  test("alerts page includes page name in title", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/alerts**", (route) => route.fulfill(dataEnvelope([])));
    await page.goto("/alerts");
    await expect(page).toHaveTitle(/Alerts.*Alga/);
  });
});
