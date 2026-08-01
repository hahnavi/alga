import { expect, test } from "@playwright/test";
import {
  json,
  makeDashboardStats,
  makeIncidentMetrics,
  makeOnCallEntry,
  mockAuthenticated,
  mockDashboardApis,
} from "./helpers";

test.describe("dashboard: overview", () => {
  test("renders KPI cards from stats and metrics", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page, {
      stats: makeDashboardStats({
        alerts: { total: 120, firing: 5, resolved: 115, unacknowledged: 3 },
        incidents: {
          total: 30,
          active: 2,
          mitigated: 1,
          resolved: 27,
          by_severity: { critical: 4 },
          by_priority: { P1: 2 },
        },
      }),
      metrics: makeIncidentMetrics({ mtta_minutes: 12.5, mttr_minutes: 95 }),
    });

    await page.goto("/");
    await expect(page.getByText("12.5 min")).toBeVisible();
    await expect(page.getByText("1h 35m")).toBeVisible();
    await expect(page.getByText("MTTA").first()).toBeVisible();
    await expect(page.getByText("MTTR").first()).toBeVisible();
    await expect(page.getByText("Active Incidents")).toBeVisible();
    await expect(page.getByText("97.5%")).toBeVisible();
  });

  test("lists active incidents and navigates to detail on click", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page);

    await page.goto("/");
    const row = page.getByText("Payment API latency").first();
    await expect(row).toBeVisible();
    await row.click();
    await expect(page).toHaveURL(/\/incidents\/21/);
  });

  test("shows on-call entries from who-is-on-call", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page, { onCall: [makeOnCallEntry()] });

    await page.goto("/");
    await expect(page.getByText("On-Call Now")).toBeVisible();
    await expect(page.getByText("Priya Sharma")).toBeVisible();
    await expect(page.getByText("Platform Team")).toBeVisible();
  });

  test("shows empty states when nothing is active", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page, {
      stats: makeDashboardStats({ active_incidents: [], active_investigations: [] }),
      onCall: [],
      services: { items: [], total: 0 },
    });

    await page.goto("/");
    await expect(page.getByText(/no active incidents or investigations/i)).toBeVisible();
    await expect(page.getByText(/no one currently on call/i)).toBeVisible();
    await expect(page.getByText(/no services configured/i)).toBeVisible();
  });

  test("shows error banner when stats request fails", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page);
    await page.route("**/api/v1/dashboard/stats", (route) =>
      route.fulfill({ status: 500, ...json({ error: "Internal server error" }) }),
    );

    await page.goto("/");
    await expect(page.getByText(/error|failed|something went wrong/i).first()).toBeVisible();
  });
});

test.describe("dashboard: date range", () => {
  test("switching range refetches incident metrics with new window", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page);

    const metricUrls: string[] = [];
    page.on("request", (req) => {
      if (req.url().includes("/api/v1/incidents/metrics")) metricUrls.push(req.url());
    });

    await page.goto("/");
    await expect(page.getByText("MTTA").first()).toBeVisible();
    await page.getByRole("button", { name: "7d", exact: true }).click();

    await expect.poll(() => metricUrls.length).toBeGreaterThanOrEqual(2);
    const first = new URL(metricUrls[0]).searchParams.get("start_date");
    const last = new URL(metricUrls[metricUrls.length - 1]).searchParams.get("start_date");
    expect(first).not.toBeNull();
    expect(last).not.toBeNull();
    expect(last).not.toBe(first);
  });
});

test.describe("dashboard: daily write-up", () => {
  test("expanding the write-up shows the generated summary", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page, {
      summary: {
        summary: "## Overnight\n\nTwo alerts fired on payments.",
        generated_at: "2025-06-01T08:00:00Z",
        period: "24h",
        available: true,
      },
    });

    await page.goto("/");
    const summaryToggle = page.getByRole("button", { name: /daily write-up/i });
    await expect(summaryToggle).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByText(/Two alerts fired on payments/i)).toBeVisible();
    await expect(page.getByText(/generated/i)).toBeVisible();
  });

  test("unavailable summary explains LLM configuration", async ({ page }) => {
    await mockAuthenticated(page);
    await mockDashboardApis(page);

    await page.goto("/");
    await page.getByRole("button", { name: /daily write-up/i }).click();
    await expect(page.getByText(/AI Summary Unavailable/i)).toBeVisible();
  });
});
