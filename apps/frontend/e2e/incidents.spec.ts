import { expect, test } from "@playwright/test";
import {
  dataEnvelope,
  json,
  makeIncident,
  mockAuthenticated,
  mockIncidentDetailApis,
  VIEWER_USER,
} from "./helpers";

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

  test("shows total count in summary line", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [
            makeIncident({ incident_number: 1 }),
            makeIncident({ incident_number: 2, title: "Second" }),
            makeIncident({ incident_number: 3, title: "Third" }),
          ],
          total: 3,
        }),
      ),
    );

    await page.goto("/incidents");
    await expect(page.getByText("3 incidents")).toBeVisible();
  });

  test("shows singular count for one incident", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [makeIncident({ incident_number: 1 })],
          total: 1,
        }),
      ),
    );

    await page.goto("/incidents");
    await expect(page.getByText("1 incident")).toBeVisible();
  });
});

test.describe("incidents: list filtering", () => {
  test("status filter sends filtered request", async ({ page }) => {
    await mockAuthenticated(page);
    const requestedUrls: string[] = [];
    await page.route("**/api/v1/incidents**", (route) => {
      requestedUrls.push(route.request().url());
      return route.fulfill(
        dataEnvelope({ items: [makeIncident({ incident_number: 1 })], total: 1 }),
      );
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();

    await page.getByLabel("Toggle filters").click();
    await page.getByLabel("Filter by incident status").selectOption("active");
    await expect.poll(() => requestedUrls.some((u) => u.includes("status=active"))).toBe(true);
  });

  test("priority filter sends filtered request", async ({ page }) => {
    await mockAuthenticated(page);
    const requestedUrls: string[] = [];
    await page.route("**/api/v1/incidents**", (route) => {
      requestedUrls.push(route.request().url());
      return route.fulfill(
        dataEnvelope({ items: [makeIncident({ incident_number: 1 })], total: 1 }),
      );
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();

    await page.getByLabel("Toggle filters").click();
    await page.getByLabel("Filter by incident priority").selectOption("P1");
    await expect.poll(() => requestedUrls.some((u) => u.includes("priority=P1"))).toBe(true);
  });

  test("search input filters incidents", async ({ page }) => {
    await mockAuthenticated(page);
    const requestedUrls: string[] = [];
    await page.route("**/api/v1/incidents**", (route) => {
      requestedUrls.push(route.request().url());
      return route.fulfill(
        dataEnvelope({ items: [makeIncident({ incident_number: 1 })], total: 1 }),
      );
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();

    await page.getByLabel("Search").click();
    await page.getByPlaceholder("Search incidents...").fill("database");
    await expect.poll(() => requestedUrls.some((u) => u.includes("search=database"))).toBe(true);
  });

  test("sort select sends sort parameter", async ({ page }) => {
    await mockAuthenticated(page);
    const requestedUrls: string[] = [];
    await page.route("**/api/v1/incidents**", (route) => {
      requestedUrls.push(route.request().url());
      return route.fulfill(
        dataEnvelope({ items: [makeIncident({ incident_number: 1 })], total: 1 }),
      );
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();

    await page.getByLabel("Toggle filters").click();
    await page.getByLabel("Sort by").selectOption("severity");
    await expect.poll(() => requestedUrls.some((u) => u.includes("sort="))).toBe(true);
  });

  test("clear filters resets to default state", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(dataEnvelope({ items: [makeIncident({ incident_number: 1 })], total: 1 })),
    );
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents?status=active&priority=P1");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();

    await page.getByLabel("Toggle filters").click();
    await page.getByRole("button", { name: /clear filters/i }).click();
    await expect(page.getByLabel("Filter by incident status")).toHaveValue("all");
    await expect(page.getByLabel("Filter by incident priority")).toHaveValue("all");
  });

  test("filters are restored from URL on load", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(dataEnvelope({ items: [makeIncident({ incident_number: 1 })], total: 1 })),
    );
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents?status=mitigated&priority=P2");
    await page.getByLabel("Toggle filters").click();
    await expect(page.getByLabel("Filter by incident status")).toHaveValue("mitigated");
    await expect(page.getByLabel("Filter by incident priority")).toHaveValue("P2");
  });

  test("invalid filter values in URL fall back to defaults", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(dataEnvelope({ items: [makeIncident({ incident_number: 1 })], total: 1 })),
    );
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents?status=bogus&priority=ZZZ");
    await page.getByLabel("Toggle filters").click();
    await expect(page.getByLabel("Filter by incident status")).toHaveValue("all");
    await expect(page.getByLabel("Filter by incident priority")).toHaveValue("all");
  });
});

test.describe("incidents: list pagination", () => {
  test("load more fetches next page and appends", async ({ page }) => {
    await mockAuthenticated(page);
    const requestedUrls: string[] = [];
    await page.route("**/api/v1/incidents**", (route) => {
      const url = route.request().url();
      requestedUrls.push(url);
      if (url.includes("skip=50")) {
        return route.fulfill(
          dataEnvelope({
            items: Array.from({ length: 25 }, (_, i) =>
              makeIncident({ incident_number: i + 51, title: `Incident ${i + 51}` }),
            ),
            total: 75,
          }),
        );
      }
      return route.fulfill(
        dataEnvelope({
          items: Array.from({ length: 50 }, (_, i) =>
            makeIncident({ incident_number: i + 1, title: `Incident ${i + 1}` }),
          ),
          total: 75,
        }),
      );
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents");
    await expect(page.getByText("Incident 50", { exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Load More" }).click();
    await expect(page.getByText("Incident 51", { exact: true })).toBeVisible();
    await expect.poll(() => requestedUrls.some((u) => u.includes("skip=50"))).toBe(true);
  });

  test("load more button hidden when all items loaded", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [makeIncident({ incident_number: 1 })],
          total: 1,
        }),
      ),
    );

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();
    await expect(page.getByRole("button", { name: "Load More" })).not.toBeVisible();
  });
});

test.describe("incidents: list actions", () => {
  test("resolve action from list sends POST and updates row", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 20, status: "active" });
    await page.route("**/api/v1/incidents**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope({ items: [incident], total: 1 }));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    let resolveCalled = false;
    await page.route("**/api/v1/incidents/20/resolve", (route) => {
      resolveCalled = true;
      return route.fulfill(
        dataEnvelope({ incident: { ...incident, status: "resolved" }, cascade: {} }),
      );
    });

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();
    await page.getByLabel("Incident actions").click();
    await page.getByRole("menuitem", { name: "Resolve" }).click();

    await expect.poll(() => resolveCalled).toBe(true);
    await expect(page.getByText("Resolved").first()).toBeVisible();
  });

  test("close action from list sends POST and updates row", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 21, status: "resolved" });
    await page.route("**/api/v1/incidents**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope({ items: [incident], total: 1 }));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    let closeCalled = false;
    await page.route("**/api/v1/incidents/21/close", (route) => {
      closeCalled = true;
      return route.fulfill(
        dataEnvelope({ incident: { ...incident, status: "closed" }, cascade: {} }),
      );
    });

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();
    await page.getByLabel("Incident actions").click();
    await page.getByRole("menuitem", { name: "Close" }).click();

    await expect.poll(() => closeCalled).toBe(true);
    await expect(page.getByText("Closed").first()).toBeVisible();
  });

  test("reopen action from list sends POST and updates row", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 22, status: "closed" });
    await page.route("**/api/v1/incidents**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope({ items: [incident], total: 1 }));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    let reopenCalled = false;
    await page.route("**/api/v1/incidents/22/reopen", (route) => {
      reopenCalled = true;
      return route.fulfill(dataEnvelope({ ...incident, status: "active" }));
    });

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();
    await page.getByLabel("Incident actions").click();
    await page.getByRole("menuitem", { name: "Reopen" }).click();

    await expect.poll(() => reopenCalled).toBe(true);
    await expect(page.getByText("Active").first()).toBeVisible();
  });

  test("delete action shows confirm dialog and removes row", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 23, status: "closed" });
    await page.route("**/api/v1/incidents**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope({ items: [incident], total: 1 }));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    let deleteCalled = false;
    await page.route("**/api/v1/incidents/23", (route) => {
      if (route.request().method() === "DELETE") {
        deleteCalled = true;
        return route.fulfill(dataEnvelope({ status: "ok" }));
      }
      return route.fulfill(dataEnvelope({ items: [incident], total: 1 }));
    });

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();
    await page.getByLabel("Incident actions").click();
    await page.getByRole("menuitem", { name: "Delete" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(/are you sure/i)).toBeVisible();
    await dialog.getByRole("button", { name: "Delete" }).click();

    await expect.poll(() => deleteCalled).toBe(true);
    await expect(page.getByText("Database connection pool exhausted")).not.toBeVisible();
  });

  test("action failure shows error toast", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 24, status: "active" });
    await page.route("**/api/v1/incidents**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope({ items: [incident], total: 1 }));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.route("**/api/v1/incidents/24/resolve", (route) =>
      route.fulfill({ status: 500, ...json({ error: "Internal server error" }) }),
    );

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();
    await page.getByLabel("Incident actions").click();
    await page.getByRole("menuitem", { name: "Resolve" }).click();

    await expect(page.getByText(/action failed|error/i).first()).toBeVisible();
  });

  test("action removes row when updated incident no longer passes filters", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 25, status: "active" });
    await page.route("**/api/v1/incidents**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope({ items: [incident], total: 1 }));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.route("**/api/v1/incidents/25/resolve", (route) =>
      route.fulfill(dataEnvelope({ incident: { ...incident, status: "resolved" }, cascade: {} })),
    );

    await page.goto("/incidents?status=active");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();
    await page.getByLabel("Incident actions").click();
    await page.getByRole("menuitem", { name: "Resolve" }).click();

    await expect(page.getByText("Database connection pool exhausted")).not.toBeVisible();
  });
});

test.describe("incidents: list RBAC", () => {
  test("viewer role does not see action buttons", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [makeIncident({ incident_number: 1, status: "active" })],
          total: 1,
        }),
      ),
    );

    await page.goto("/incidents");
    await expect(page.getByText("Database connection pool exhausted")).toBeVisible();
    await expect(page.getByLabel("Incident actions")).not.toBeVisible();
  });

  test("viewer role does not see create button", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    await page.goto("/incidents");
    await expect(
      page.getByRole("button", { name: "Create incident", exact: true }),
    ).not.toBeVisible();
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

  test("create dialog shows computed priority from severity and impact", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );
    await page.route("**/api/v1/services**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    await page.goto("/incidents");
    await page.getByRole("button", { name: "Create incident", exact: true }).click();
    const dialog = page.getByRole("dialog");

    await dialog.getByLabel("Severity").selectOption("critical");
    await dialog.getByLabel("Impact").selectOption("high");
    await expect(dialog.getByText("P1")).toBeVisible();
  });

  test("create dialog requires title", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );
    await page.route("**/api/v1/services**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    await page.goto("/incidents");
    await page.getByRole("button", { name: "Create incident", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: /create incident/i }).click();
    await expect(dialog.getByText(/title is required/i)).toBeVisible();
  });

  test("create dialog submits severity, impact, and priority", async ({ page }) => {
    await mockAuthenticated(page);
    let postBody: Record<string, unknown> = {};
    await page.route("**/api/v1/incidents", (route) => {
      if (route.request().method() === "POST") {
        postBody = route.request().postDataJSON();
        return route.fulfill(dataEnvelope(makeIncident({ incident_number: 99 })));
      }
      return route.fulfill(dataEnvelope({ items: [], total: 0 }));
    });
    await page.route("**/api/v1/services**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    await page.goto("/incidents");
    await page.getByRole("button", { name: "Create incident", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel(/title/i).fill("Severity test");
    await dialog.getByLabel("Severity").selectOption("high");
    await dialog.getByLabel("Impact").selectOption("medium");
    await dialog.getByRole("button", { name: /create incident/i }).click();

    await expect.poll(() => postBody.title).toBe("Severity test");
    expect(postBody.severity).toBe("high");
    expect(postBody.impact_level).toBe("medium");
    expect(postBody.priority).toBe("P3");
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

  test("shows loading skeleton then content", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({
      incident_number: 30,
      title: "SkeletonTest Incident",
    });
    await mockIncidentDetailApis(page, 30, { incident });

    await page.goto("/incidents/30");
    await expect(page.getByText("SkeletonTest Incident").first()).toBeVisible();
    await expect(page.locator("[aria-busy='true']")).not.toBeVisible();
  });

  test("shows error state on API failure", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents/31**", (route) =>
      route.fulfill({ status: 500, ...json({ error: "Internal server error" }) }),
    );
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents/31");
    await expect(page.getByText(/error|failed|something went wrong/i).first()).toBeVisible();
  });

  test("deleted incident shows read-only banner", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({
      incident_number: 32,
      deleted_at: "2025-06-02T00:00:00Z",
    });
    await mockIncidentDetailApis(page, 32, { incident });

    await page.goto("/incidents/32");
    await expect(page.getByText(/deleted.*read-only/i)).toBeVisible();
  });

  test("shows SSE disconnected banner when stream is unavailable", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 33 });
    await mockIncidentDetailApis(page, 33, { incident });

    await page.goto("/incidents/33");
    await expect(page.getByText(/live updates paused/i)).toBeVisible();
  });

  test("displays priority, severity, and impact badges", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({
      incident_number: 34,
      priority: "P1",
      severity: "critical",
      impact_level: "high",
    });
    await mockIncidentDetailApis(page, 34, { incident });

    await page.goto("/incidents/34");
    await expect(page.getByText("P1").first()).toBeVisible();
    await expect(page.getByText("critical").first()).toBeVisible();
    await expect(page.getByText("high").first()).toBeVisible();
  });

  test("displays description and tags", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({
      incident_number: 35,
      description: "Primary database is unreachable",
    });
    await mockIncidentDetailApis(page, 35, { incident });

    await page.goto("/incidents/35");
    await expect(page.getByText("Primary database is unreachable")).toBeVisible();
  });
});

test.describe("incidents: detail status transitions", () => {
  test("resolve updates status badge to Resolved", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 40, status: "active" });
    await mockIncidentDetailApis(page, 40, { incident });

    await page.route("**/api/v1/incidents/40/resolve", (route) =>
      route.fulfill(dataEnvelope({ incident: { ...incident, status: "resolved" }, cascade: {} })),
    );

    await page.goto("/incidents/40");
    await page.getByRole("button", { name: "Incident actions" }).click();
    await page.getByRole("menuitem", { name: "Resolve" }).click();

    await expect(page.getByText("Resolved").first()).toBeVisible();
  });

  test("close updates status badge to Closed", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 41, status: "resolved" });
    await mockIncidentDetailApis(page, 41, { incident });

    await page.route("**/api/v1/incidents/41/close", (route) =>
      route.fulfill(dataEnvelope({ incident: { ...incident, status: "closed" }, cascade: {} })),
    );

    await page.goto("/incidents/41");
    await page.getByRole("button", { name: "Incident actions" }).click();
    await page.getByRole("menuitem", { name: "Close" }).click();

    await expect(page.getByText("Closed").first()).toBeVisible();
  });

  test("reopen updates status badge to Active", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 42, status: "closed" });
    await mockIncidentDetailApis(page, 42, { incident });

    await page.route("**/api/v1/incidents/42/reopen", (route) =>
      route.fulfill(dataEnvelope({ ...incident, status: "active" })),
    );

    await page.goto("/incidents/42");
    await page.getByRole("button", { name: "Incident actions" }).click();
    await page.getByRole("menuitem", { name: "Reopen" }).click();

    await expect(page.getByText("Active").first()).toBeVisible();
  });

  test("acknowledge action sends POST", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 43, status: "detected" });
    await mockIncidentDetailApis(page, 43, { incident });

    let ackCalled = false;
    await page.route("**/api/v1/incidents/43/acknowledge", (route) => {
      ackCalled = true;
      return route.fulfill(dataEnvelope({ ...incident, status: "triaging" }));
    });

    await page.goto("/incidents/43");
    await page.getByRole("button", { name: "Incident actions" }).click();
    await page.getByRole("menuitem", { name: "Acknowledge" }).click();

    await expect.poll(() => ackCalled).toBe(true);
  });

  test("escalate action sends POST for active incident", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 44, status: "active" });
    await mockIncidentDetailApis(page, 44, { incident });

    let escalateCalled = false;
    await page.route("**/api/v1/incidents/44/escalate", (route) => {
      escalateCalled = true;
      return route.fulfill(dataEnvelope({ ...incident, status: "active" }));
    });

    await page.goto("/incidents/44");
    await page.getByRole("button", { name: "Incident actions" }).click();
    await page.getByRole("menuitem", { name: "Escalate" }).click();

    await expect.poll(() => escalateCalled).toBe(true);
  });

  test("delete incident navigates back to list", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 45, status: "closed" });
    await mockIncidentDetailApis(page, 45, { incident });
    await page.route("**/api/v1/incidents**", (route) => {
      if (route.request().method() === "GET" && !route.request().url().includes("/incidents/45")) {
        return route.fulfill(dataEnvelope({ items: [], total: 0 }));
      }
      return route.fallback();
    });

    let deleteCalled = false;
    await page.route("**/api/v1/incidents/45", (route) => {
      if (route.request().method() === "DELETE") {
        deleteCalled = true;
        return route.fulfill(dataEnvelope({ status: "ok" }));
      }
      return route.fallback();
    });

    await page.goto("/incidents/45");
    await page.getByRole("button", { name: "Incident actions" }).click();
    await page.getByRole("menuitem", { name: "Delete" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Delete" }).click();

    await expect.poll(() => deleteCalled).toBe(true);
    await expect(page).toHaveURL(/\/incidents$/);
  });
});

test.describe("incidents: detail editing", () => {
  test("edit dialog sends PATCH with updated fields", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 50, title: "Original title" });
    await mockIncidentDetailApis(page, 50, { incident });

    let patchBody: Record<string, unknown> = {};
    await page.route("**/api/v1/incidents/50", (route) => {
      if (route.request().method() === "PATCH") {
        patchBody = route.request().postDataJSON();
        return route.fulfill(dataEnvelope({ ...incident, title: "Updated title" }));
      }
      return route.fallback();
    });

    await page.goto("/incidents/50");
    await page.getByRole("button", { name: "Incident actions" }).click();
    await page.getByRole("menuitem", { name: "Edit" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const titleInput = dialog.locator("#edit-incident-title-input");
    await titleInput.clear();
    await titleInput.fill("Updated title");
    await dialog.getByRole("button", { name: "Save" }).click();

    await expect.poll(() => patchBody.title).toBe("Updated title");
  });

  test("add timeline entry sends POST", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 51 });
    await mockIncidentDetailApis(page, 51, { incident });

    let timelineBody: Record<string, unknown> = {};
    await page.route("**/api/v1/incidents/51/timeline", (route) => {
      if (route.request().method() === "POST") {
        timelineBody = route.request().postDataJSON();
        return route.fulfill(
          dataEnvelope({
            id: "tl-1",
            incident_number: 51,
            event_type: "manual",
            actor_type: "user",
            message: "Investigation started",
            metadata: {},
            created_at: "2025-06-01T10:00:00Z",
          }),
        );
      }
      return route.fallback();
    });

    await page.goto("/incidents/51");
    await page.getByTitle("Add timeline entry").click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.locator("#timeline-message").fill("Investigation started");
    await dialog.getByRole("button", { name: "Add" }).click();

    await expect.poll(() => timelineBody.message).toBe("Investigation started");
  });

  test("link alert dialog opens and accepts alert number", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 52 });
    await mockIncidentDetailApis(page, 52, { incident });

    await page.goto("/incidents/52");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await page.getByRole("button", { name: "Link" }).first().click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Alert Number")).toBeVisible();
    const input = dialog.locator("#link-alert-number");
    await input.fill("42");
    await expect(input).toHaveValue("42");
  });

  test("unlink alert shows confirm dialog", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 53 });
    const linkedAlert = {
      fingerprint: "fp-linked-1",
      alert_number: 100,
      status: "firing",
      acknowledged: false,
      silenced: false,
      labels: { alertname: "LinkedAlert", severity: "critical" },
      annotations: {},
      values: null,
      starts_at: "2025-06-01T10:00:00Z",
      ends_at: null,
      generator_url: "",
      events: [],
      updated_at: "2025-06-01T10:00:00Z",
      created_at: "2025-06-01T10:00:00Z",
      deleted_at: null,
    };
    await mockIncidentDetailApis(page, 53, { incident, alerts: [linkedAlert] });

    await page.goto("/incidents/53");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await expect(page.getByText("LinkedAlert")).toBeVisible();
    await page.getByTitle("Unlink alert").click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(/remove this alert/i)).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Unlink" })).toBeVisible();
  });
});

test.describe("incidents: detail document sections", () => {
  test("summary section shows empty state and edit flow", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 60 });
    await mockIncidentDetailApis(page, 60, { incident, document: [] });

    await page.goto("/incidents/60");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await expect(page.getByText("No executive summary recorded.")).toBeVisible();

    await page.getByTitle("Edit summary").click();

    let putCalled = false;
    await page.route("**/api/v1/incidents/60**", (route) => {
      if (route.request().method() === "PATCH") {
        putCalled = true;
        return route.fulfill(dataEnvelope({ ...incident, summary: "Test summary" }));
      }
      return route.fallback();
    });

    await page.getByRole("button", { name: "Save" }).first().click();
    await expect.poll(() => putCalled).toBe(true);
  });

  test("root cause section shows empty state", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 61 });
    await mockIncidentDetailApis(page, 61, { incident, document: [] });

    await page.goto("/incidents/61");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await expect(page.getByText("No root cause recorded.")).toBeVisible();
  });

  test("resolution section shows empty state", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 62 });
    await mockIncidentDetailApis(page, 62, { incident, document: [] });

    await page.goto("/incidents/62");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await expect(page.getByText("No resolution recorded.")).toBeVisible();
  });

  test("impact section shows empty state", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 63 });
    await mockIncidentDetailApis(page, 63, { incident, document: [] });

    await page.goto("/incidents/63");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await expect(page.getByText("No impact assessment recorded.")).toBeVisible();
  });
});

test.describe("incidents: detail linked alerts", () => {
  test("shows linked alerts with status badges", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 70 });
    const alerts = [
      {
        fingerprint: "fp-70-1",
        alert_number: 200,
        status: "firing",
        acknowledged: false,
        silenced: false,
        labels: { alertname: "HighMemory", severity: "critical" },
        annotations: {},
        values: null,
        starts_at: "2025-06-01T10:00:00Z",
        ends_at: null,
        generator_url: "",
        events: [],
        updated_at: "2025-06-01T10:00:00Z",
        created_at: "2025-06-01T10:00:00Z",
        deleted_at: null,
      },
    ];
    await mockIncidentDetailApis(page, 70, { incident, alerts });

    await page.goto("/incidents/70");
    await expect(page.getByText("HighMemory")).toBeVisible();
    await expect(page.getByText("#200")).toBeVisible();
  });

  test("shows empty state when no linked alerts", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 71 });
    await mockIncidentDetailApis(page, 71, { incident, alerts: [] });

    await page.goto("/incidents/71");
    await expect(page.getByText("No linked alerts.")).toBeVisible();
  });
});

test.describe("incidents: detail coordination thread", () => {
  test("opens and closes coordination thread panel", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 80 });
    await mockIncidentDetailApis(page, 80, { incident });

    await page.goto("/incidents/80");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible({
      timeout: 10000,
    });
    await page.locator('button[aria-controls="incident-thread-drawer"]').first().click();
    await expect(page.locator("#incident-coordination-drawer-title")).toBeVisible();

    await page.getByLabel("Close coordination thread").click();
    await expect(page.locator("#incident-coordination-drawer-title")).not.toBeVisible();
  });

  test("opens investigation thread panel", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 81 });
    await mockIncidentDetailApis(page, 81, { incident });

    await page.goto("/incidents/81");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await page.locator('button[aria-controls="incident-thread-drawer"]').last().click();
    await expect(page.getByLabel("Incident investigation thread")).toBeVisible();
  });

  test("coordination messages render in thread", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 82 });
    const messages = [
      {
        id: "msg-1",
        incident_number: 82,
        kind: "chat",
        body: "Checking the database connections now",
        internal: false,
        source: "user",
        user_id: "user-admin-1",
        username: "Admin User",
        created_at: "2025-06-01T10:05:00Z",
        updated_at: "2025-06-01T10:05:00Z",
      },
    ];
    await mockIncidentDetailApis(page, 82, { incident, coordinationMessages: messages });

    await page.goto("/incidents/82");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await page.locator('button[aria-controls="incident-thread-drawer"]').first().click();
    await expect(page.getByText("Checking the database connections now")).toBeVisible();
  });
});

test.describe("incidents: SSE live updates (detail)", () => {
  test("incident_updated SSE triggers reload of detail", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({
      incident_number: 90,
      title: "Before SSE Update",
    });

    let getRequestCount = 0;
    await page.route(`**/api/v1/incidents/90/alerts**`, (route) => route.fulfill(dataEnvelope([])));
    await page.route(`**/api/v1/incidents/90/ics/roles**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/90/ics/document**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/90/coordination/messages**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/90/coordination/tasks**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/90/status-updates**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/90/thread**`, (route) =>
      route.fulfill(dataEnvelope({ messages: [], thread_id: "t-1", provider: "internal" })),
    );
    await page.route(`**/api/v1/incidents/90/timeline**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/90/post-mortem**`, (route) =>
      route.fulfill({ status: 404, ...json({ error: "Not found" }) }),
    );
    await page.route(`**/api/v1/incidents/90`, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      getRequestCount++;
      return route.fulfill(dataEnvelope({ incident }));
    });
    await page.route("**/api/v1/users**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/agent-tokens**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/integrations**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/playbooks**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    let releaseStream!: () => void;
    const streamGate = new Promise<void>((resolve) => {
      releaseStream = resolve;
    });
    await page.route("**/api/v1/events**", async (route) => {
      await streamGate;
      try {
        await route.fulfill({
          contentType: "text/event-stream",
          body: `retry: 30000\nevent: incident_updated\ndata: ${JSON.stringify({ incident_number: 90 })}\n\n`,
        });
      } catch {
        // aborted on navigation
      }
    });

    await page.goto("/incidents/90");
    await expect(page.getByText("Before SSE Update").first()).toBeVisible();
    const countBefore = getRequestCount;
    releaseStream();

    await expect.poll(() => getRequestCount, { timeout: 10000 }).toBeGreaterThan(countBefore);
  });

  test("incident_status_changed SSE triggers reload", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 91, status: "active" });

    let getRequestCount = 0;
    await page.route(`**/api/v1/incidents/91/alerts**`, (route) => route.fulfill(dataEnvelope([])));
    await page.route(`**/api/v1/incidents/91/ics/roles**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/91/ics/document**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/91/coordination/messages**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/91/coordination/tasks**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/91/status-updates**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/91/thread**`, (route) =>
      route.fulfill(dataEnvelope({ messages: [], thread_id: "t-1", provider: "internal" })),
    );
    await page.route(`**/api/v1/incidents/91/timeline**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/91/post-mortem**`, (route) =>
      route.fulfill({ status: 404, ...json({ error: "Not found" }) }),
    );
    await page.route(`**/api/v1/incidents/91`, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      getRequestCount++;
      return route.fulfill(dataEnvelope({ incident }));
    });
    await page.route("**/api/v1/users**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/agent-tokens**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/integrations**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/playbooks**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    let releaseStream!: () => void;
    const streamGate = new Promise<void>((resolve) => {
      releaseStream = resolve;
    });
    await page.route("**/api/v1/events**", async (route) => {
      await streamGate;
      try {
        await route.fulfill({
          contentType: "text/event-stream",
          body: `retry: 30000\nevent: incident_status_changed\ndata: ${JSON.stringify({ incident_number: 91, status: "mitigated" })}\n\n`,
        });
      } catch {
        // aborted on navigation
      }
    });

    await page.goto("/incidents/91");
    await expect(page.getByText("Active").first()).toBeVisible();
    const countBefore = getRequestCount;
    releaseStream();

    await expect.poll(() => getRequestCount, { timeout: 10000 }).toBeGreaterThan(countBefore);
  });

  test("SSE event for different incident does not trigger reload", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 92 });

    let getRequestCount = 0;
    await page.route(`**/api/v1/incidents/92/alerts**`, (route) => route.fulfill(dataEnvelope([])));
    await page.route(`**/api/v1/incidents/92/ics/roles**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/92/ics/document**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/92/coordination/messages**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/92/coordination/tasks**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/92/status-updates**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/92/thread**`, (route) =>
      route.fulfill(dataEnvelope({ messages: [], thread_id: "t-1", provider: "internal" })),
    );
    await page.route(`**/api/v1/incidents/92/timeline**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/92/post-mortem**`, (route) =>
      route.fulfill({ status: 404, ...json({ error: "Not found" }) }),
    );
    await page.route(`**/api/v1/incidents/92`, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      getRequestCount++;
      return route.fulfill(dataEnvelope({ incident }));
    });
    await page.route("**/api/v1/users**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/agent-tokens**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/integrations**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/playbooks**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    let releaseStream!: () => void;
    const streamGate = new Promise<void>((resolve) => {
      releaseStream = resolve;
    });
    await page.route("**/api/v1/events**", async (route) => {
      await streamGate;
      try {
        await route.fulfill({
          contentType: "text/event-stream",
          body: `retry: 30000\nevent: incident_updated\ndata: ${JSON.stringify({ incident_number: 999 })}\n\n`,
        });
      } catch {
        // aborted on navigation
      }
    });

    await page.goto("/incidents/92");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    const countBefore = getRequestCount;
    releaseStream();

    await page.waitForTimeout(3000);
    expect(getRequestCount).toBe(countBefore);
  });

  test("incident_coordination_message_created SSE reloads coordination messages", async ({
    page,
  }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 93 });

    let coordMsgRequestCount = 0;
    await page.route(`**/api/v1/incidents/93/alerts**`, (route) => route.fulfill(dataEnvelope([])));
    await page.route(`**/api/v1/incidents/93/ics/roles**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/93/ics/document**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/93/coordination/messages**`, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      coordMsgRequestCount++;
      return route.fulfill(dataEnvelope([]));
    });
    await page.route(`**/api/v1/incidents/93/coordination/tasks**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/93/status-updates**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/93/thread**`, (route) =>
      route.fulfill(dataEnvelope({ messages: [], thread_id: "t-1", provider: "internal" })),
    );
    await page.route(`**/api/v1/incidents/93/timeline**`, (route) =>
      route.fulfill(dataEnvelope([])),
    );
    await page.route(`**/api/v1/incidents/93/post-mortem**`, (route) =>
      route.fulfill({ status: 404, ...json({ error: "Not found" }) }),
    );
    await page.route(`**/api/v1/incidents/93`, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope({ incident }));
    });
    await page.route("**/api/v1/users**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/agent-tokens**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/integrations**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/playbooks**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    let releaseStream!: () => void;
    const streamGate = new Promise<void>((resolve) => {
      releaseStream = resolve;
    });
    await page.route("**/api/v1/events**", async (route) => {
      await streamGate;
      try {
        await route.fulfill({
          contentType: "text/event-stream",
          body: `retry: 30000\nevent: incident_coordination_message_created\ndata: ${JSON.stringify({ incident_number: 93 })}\n\n`,
        });
      } catch {
        // aborted on navigation
      }
    });

    await page.goto("/incidents/93");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    const countBefore = coordMsgRequestCount;
    releaseStream();

    await expect.poll(() => coordMsgRequestCount, { timeout: 10000 }).toBeGreaterThan(countBefore);
  });
});

test.describe("incidents: SSE live updates (list)", () => {
  test("incident list does not have SSE-driven updates (no live list)", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill(
        dataEnvelope({
          items: [makeIncident({ incident_number: 1, title: "Static incident" })],
          total: 1,
        }),
      ),
    );
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/incidents");
    await expect(page.getByText("Static incident")).toBeVisible();
    await expect(page.getByText(/live updates paused/i)).not.toBeVisible();
  });
});

test.describe("incidents: detail timeline", () => {
  test("timeline section is collapsed by default and expands on click", async ({ page }) => {
    await mockAuthenticated(page);
    const incident = makeIncident({ incident_number: 100 });
    await mockIncidentDetailApis(page, 100, { incident });

    await page.goto("/incidents/100");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    const timelineToggle = page.locator('button[aria-controls="incident-timeline-body"]');
    await expect(timelineToggle).toBeVisible();
    await expect(timelineToggle).toHaveAttribute("aria-expanded", "false");

    await timelineToggle.click();
    await expect(timelineToggle).toHaveAttribute("aria-expanded", "true");
  });
});

test.describe("incidents: detail RBAC", () => {
  test("viewer role does not see command actions in menu", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    const incident = makeIncident({ incident_number: 110, status: "active" });
    await mockIncidentDetailApis(page, 110, { incident });

    await page.goto("/incidents/110");
    await expect(page.getByText("Database connection pool exhausted").first()).toBeVisible();
    await page.getByRole("button", { name: "Incident actions" }).click();
    await expect(page.getByRole("menuitem", { name: "Mitigate" })).not.toBeVisible();
    await expect(page.getByRole("menuitem", { name: "Resolve" })).not.toBeVisible();
    await expect(page.getByRole("menuitem", { name: "Delete" })).not.toBeVisible();
  });

  test("viewer role does not see edit buttons on document sections", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    const incident = makeIncident({ incident_number: 111 });
    await mockIncidentDetailApis(page, 111, { incident, document: [] });

    await page.goto("/incidents/111");
    await expect(page.getByTitle("Edit summary")).not.toBeVisible();
    await expect(page.getByTitle("Edit root cause")).not.toBeVisible();
    await expect(page.getByTitle("Edit resolution")).not.toBeVisible();
    await expect(page.getByTitle("Edit impact assessment")).not.toBeVisible();
  });

  test("viewer role does not see link alert button", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    const incident = makeIncident({ incident_number: 112 });
    await mockIncidentDetailApis(page, 112, { incident, alerts: [] });

    await page.goto("/incidents/112");
    await expect(page.getByRole("button", { name: "Link" })).not.toBeVisible();
  });
});
