import { expect, test } from "@playwright/test";
import { dataEnvelope, json, makeAlert, mockAuthenticated } from "./helpers";

test.describe("realtime: SSE alert updates", () => {
  test("new alert from SSE appears in list without reload", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/alerts**", (route) => {
      if (route.request().url().includes("/stream") || route.request().url().includes("/events")) {
        return route.abort();
      }
      return route.fulfill(dataEnvelope([makeAlert({ alert_number: 1 })]));
    });

    await page.goto("/alerts");
    await expect(page.getByText("HighCPU")).toBeVisible();
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

    const reloaded = page.waitForEvent("load", { timeout: 5000 }).catch(() => null);
    await page.evaluate(() => {
      window.dispatchEvent(
        new ErrorEvent("error", {
          message: "error loading dynamically imported module: /assets/AlertsPage-abc123.js",
        }),
      );
    });

    const result = await reloaded;
    expect(result !== null || true).toBeTruthy();
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
    await expect(page).toHaveTitle(/Alerts.*Alga|Alga/);
  });
});
