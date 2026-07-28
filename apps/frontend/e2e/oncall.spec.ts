import { expect, test } from "@playwright/test";
import { dataEnvelope, json, makeOnCallEntry, makeSchedule, mockAuthenticated } from "./helpers";

test.describe("on-call: schedules", () => {
  test("renders schedule cards with the current on-call user", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/on-call/schedules**", (route) => {
      if (route.request().url().includes("/current")) {
        return route.fulfill(dataEnvelope(makeOnCallEntry()));
      }
      return route.fulfill(
        dataEnvelope({
          items: [
            makeSchedule(),
            makeSchedule({ id: "sched-002", team_id: "team-002", team_name: "SRE Team" }),
          ],
          total: 2,
        }),
      );
    });

    await page.goto("/on-call");
    await expect(page.getByText("Platform Team")).toBeVisible();
    await expect(page.getByText("SRE Team")).toBeVisible();
    await expect(page.getByText("Priya Sharma").first()).toBeVisible();
  });

  test("links to the schedule editor", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/on-call/**", (route) => {
      if (route.request().url().includes("/schedules/sched-001")) {
        return route.fulfill(dataEnvelope(makeSchedule()));
      }
      if (route.request().url().includes("/schedules")) {
        return route.fulfill(dataEnvelope({ items: [makeSchedule()], total: 1 }));
      }
      return route.fulfill(dataEnvelope([]));
    });

    await page.goto("/on-call");
    await page.getByText("Platform Team").click();
    await expect(page).toHaveURL(/\/on-call\/schedules\/sched-001/);
  });

  test("shows empty state when no schedules exist", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/on-call/**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    await page.goto("/on-call");
    await expect(page.getByText(/no schedules found/i)).toBeVisible();
  });

  test("shows error banner when schedules fail to load", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/on-call/**", (route) =>
      route.fulfill({ status: 500, ...json({ error: "Internal server error" }) }),
    );

    await page.goto("/on-call");
    await expect(page.getByText(/error|failed|something went wrong/i).first()).toBeVisible();
  });
});
