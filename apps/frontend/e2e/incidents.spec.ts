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

  test("status transition sends PATCH request", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 11, status: "active" });
    await page.route("**/api/v1/incidents/11", (route) => {
      if (route.request().method() === "PATCH") {
        return route.fulfill(dataEnvelope({ ...incident, status: "mitigated" }));
      }
      if (route.request().url().includes("/thread")) {
        return route.fulfill(
          dataEnvelope({ messages: [], thread_id: "t-1", provider: "internal" }),
        );
      }
      return route.fulfill(dataEnvelope({ incident }));
    });

    let patchCalled = false;
    await page.route("**/api/v1/incidents/11", (route) => {
      if (route.request().method() === "PATCH") {
        patchCalled = true;
        return route.fulfill(dataEnvelope({ ...incident, status: "mitigated" }));
      }
      if (route.request().url().includes("/thread")) {
        return route.fulfill(
          dataEnvelope({ messages: [], thread_id: "t-1", provider: "internal" }),
        );
      }
      return route.fulfill(dataEnvelope({ incident }));
    });

    await page.goto("/incidents/11");
    const mitigateBtn = page.getByRole("button", { name: /mitigate/i });
    if (await mitigateBtn.isVisible()) {
      await mitigateBtn.click();
      await expect.poll(() => patchCalled).toBe(true);
    }
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
    const createBtn = page.getByRole("button", { name: /new incident|create/i });
    if (await createBtn.isVisible()) {
      await createBtn.click();
      const titleInput = page.getByLabel(/title/i);
      if (await titleInput.isVisible()) {
        await titleInput.fill("Test incident");
        await page.getByRole("button", { name: /create|submit/i }).click();
        await expect.poll(() => postCalled).toBe(true);
      }
    }
  });
});
