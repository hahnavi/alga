import { expect, test } from "@playwright/test";
import { dataEnvelope, makeIncident, mockAuthenticated } from "./helpers";

test.describe("incidents: list", () => {
  test("renders incident rows from paginated response", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [
            makeIncident({ incident_number: 1, title: "DB pool exhausted" }),
            makeIncident({ incident_number: 2, title: "API latency spike", status: "mitigated" }),
          ],
          total: 2,
        }),
      ),
    );

    await page.goto("/incidents");
    await expect(page.getByText("DB pool exhausted")).toBeVisible();
    await expect(page.getByText("API latency spike")).toBeVisible();
  });

  test("shows empty state when no incidents", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    await page.goto("/incidents");
    await expect(page.getByText(/no incidents|empty/i)).toBeVisible();
  });

  test("displays severity and priority badges", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [makeIncident({ incident_number: 1, severity: "critical", priority: "P1" })],
          total: 1,
        }),
      ),
    );

    await page.goto("/incidents");
    await expect(page.getByText(/critical/i).first()).toBeVisible();
    await expect(page.getByText(/P1/i).first()).toBeVisible();
  });
});

test.describe("incidents: detail", () => {
  test("shows incident detail with title and status", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({
      incident_number: 10,
      title: "Kubernetes pod crash loop",
      status: "active",
    });
    await page.route("**/api/v1/incidents/10**", (route) => {
      if (route.request().url().includes("/thread")) {
        return route.fulfill(
          dataEnvelope({ messages: [], thread_id: "t-1", provider: "internal" }),
        );
      }
      return route.fulfill(dataEnvelope({ incident }));
    });

    await page.goto("/incidents/10");
    await expect(page.getByText("Kubernetes pod crash loop").first()).toBeVisible();
  });

  test("mitigate action sends POST via actions menu", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 11, status: "active" });
    await page.route("**/api/v1/incidents/11**", (route) => {
      if (route.request().url().includes("/thread")) {
        return route.fulfill(
          dataEnvelope({ messages: [], thread_id: "t-1", provider: "internal" }),
        );
      }
      return route.fulfill(dataEnvelope({ incident }));
    });

    let mitigateCalled = false;
    await page.route("**/api/v1/incidents/11/mitigate", (route) => {
      mitigateCalled = true;
      return route.fulfill(dataEnvelope({ ...incident, status: "mitigated" }));
    });

    await page.goto("/incidents/11");
    await page.getByRole("button", { name: "Incident actions" }).click();
    await page.getByRole("menuitem", { name: "Mitigate" }).click();
    await expect.poll(() => mitigateCalled).toBe(true);
  });
});

test.describe("incidents: create", () => {
  test("create incident dialog sends POST", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) => {
      if (route.request().method() === "POST") {
        return route.fulfill(dataEnvelope(makeIncident({ incident_number: 99 })));
      }
      return route.fulfill(dataEnvelope({ items: [], total: 0 }));
    });
    await page.route("**/api/v1/services**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    let postCalled = false;
    await page.route("**/api/v1/incidents", (route) => {
      if (route.request().method() === "POST") {
        postCalled = true;
        return route.fulfill(dataEnvelope(makeIncident({ incident_number: 99 })));
      }
      return route.fulfill(dataEnvelope({ items: [], total: 0 }));
    });

    await page.goto("/incidents");
    await page.getByRole("button", { name: "Create incident", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel(/title/i).fill("Test incident");
    await dialog.getByRole("button", { name: /create incident/i }).click();
    await expect.poll(() => postCalled).toBe(true);
  });
});
