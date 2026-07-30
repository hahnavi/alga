import { expect, test } from "@playwright/test";
import {
  dataEnvelope,
  makeAlert,
  makeInvestigation,
  makeUser,
  mockAlertDetailApis,
  mockAuthenticated,
  VIEWER_USER,
} from "./helpers";

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

test.describe("alerts: list filtering", () => {
  test("status filter sends filtered request", async ({ page }) => {
    await mockAuthenticated(page);
    const requestedUrls: string[] = [];
    await page.route("**/api/v1/alerts**", (route) => {
      requestedUrls.push(route.request().url());
      return route.fulfill(dataEnvelope([makeAlert({ alert_number: 1, status: "firing" })]));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/alerts");
    await expect(page.getByText("HighCPU")).toBeVisible();

    await page.getByLabel("Toggle filters").click();
    await page.getByLabel("Filter by status").selectOption("open");
    await expect.poll(() => requestedUrls.some((u) => u.includes("status=open"))).toBe(true);
  });

  test("search input filters alerts by name", async ({ page }) => {
    await mockAuthenticated(page);
    const requestedUrls: string[] = [];
    await page.route("**/api/v1/alerts**", (route) => {
      requestedUrls.push(route.request().url());
      return route.fulfill(dataEnvelope([makeAlert({ alert_number: 1 })]));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/alerts");
    await expect(page.getByText("HighCPU")).toBeVisible();

    await page.getByLabel("Search").click();
    await page.getByPlaceholder("Search alerts...").fill("MemoryLeak");
    await expect.poll(() => requestedUrls.some((u) => u.includes("search=MemoryLeak"))).toBe(true);
  });

  test("clear filters resets to default state", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/alerts**", (route) =>
      route.fulfill(dataEnvelope([makeAlert({ alert_number: 1 })])),
    );
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/alerts?status=open");
    await expect(page.getByText("HighCPU")).toBeVisible();

    await page.getByLabel("Toggle filters").click();
    await page.getByRole("button", { name: /clear filters/i }).click();
    await expect(page.getByLabel("Filter by status")).toHaveValue("all");
  });

  test("sort select sends sort parameter", async ({ page }) => {
    await mockAuthenticated(page);
    const requestedUrls: string[] = [];
    await page.route("**/api/v1/alerts**", (route) => {
      requestedUrls.push(route.request().url());
      return route.fulfill(dataEnvelope([makeAlert({ alert_number: 1 })]));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/alerts");
    await expect(page.getByText("HighCPU")).toBeVisible();

    await page.getByLabel("Toggle filters").click();
    await page.getByLabel("Sort by").selectOption("severity");
    await expect.poll(() => requestedUrls.some((u) => u.includes("sort="))).toBe(true);
  });
});

test.describe("alerts: list acknowledgment", () => {
  test("ack button sends POST and updates row badge", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({ alert_number: 5, status: "firing", acknowledged: false });
    await page.route("**/api/v1/alerts**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope([alert]));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    let ackCalled = false;
    await page.route("**/api/v1/alerts/5/acknowledge", (route) => {
      ackCalled = true;
      return route.fulfill(dataEnvelope({ ...alert, acknowledged: true }));
    });

    await page.goto("/alerts");
    await page.locator("button").filter({ hasText: "Ack" }).last().click();
    await expect.poll(() => ackCalled).toBe(true);
    await expect(page.getByText(/acknowledged/i).first()).toBeVisible();
  });

  test("ack button not shown for already-acknowledged alerts", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/alerts**", (route) =>
      route.fulfill(
        dataEnvelope([makeAlert({ alert_number: 6, status: "firing", acknowledged: true })]),
      ),
    );

    await page.goto("/alerts");
    await expect(page.getByText("HighCPU")).toBeVisible();
    await expect(page.getByRole("button", { name: /^ack$/i })).not.toBeVisible();
  });

  test("ack button not shown for viewer role", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.route("**/api/v1/alerts**", (route) =>
      route.fulfill(
        dataEnvelope([makeAlert({ alert_number: 7, status: "firing", acknowledged: false })]),
      ),
    );

    await page.goto("/alerts");
    await expect(page.getByText("HighCPU")).toBeVisible();
    await expect(page.getByRole("button", { name: /ack/i })).not.toBeVisible();
  });
});

test.describe("alerts: list resolve", () => {
  test("resolve via actions menu sends POST and updates row", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({ alert_number: 9, status: "firing", acknowledged: true });
    await page.route("**/api/v1/alerts**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope([alert]));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    let resolveCalled = false;
    await page.route("**/api/v1/alerts/9/resolve", (route) => {
      resolveCalled = true;
      return route.fulfill(dataEnvelope({ ...alert, status: "resolved" }));
    });

    await page.goto("/alerts");
    await page.getByLabel("Alert actions").click();
    await page.getByRole("menuitem", { name: "Mark resolved" }).click();
    await expect.poll(() => resolveCalled).toBe(true);
  });
});

test.describe("alerts: list delete", () => {
  test("delete via actions menu shows confirm and removes row", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({ alert_number: 10, status: "firing", acknowledged: true });
    await page.route("**/api/v1/alerts**", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope([alert]));
    });
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    let deleteCalled = false;
    await page.route("**/api/v1/alerts/10", (route) => {
      if (route.request().method() === "DELETE") {
        deleteCalled = true;
        return route.fulfill(dataEnvelope({ status: "ok" }));
      }
      return route.fulfill(dataEnvelope([alert]));
    });

    await page.goto("/alerts");
    await page.getByLabel("Alert actions").click();
    await page.getByRole("menuitem", { name: "Delete" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(/are you sure/i)).toBeVisible();
    await dialog.getByRole("button", { name: "Delete" }).click();

    await expect.poll(() => deleteCalled).toBe(true);
    await expect(page.getByText("HighCPU")).not.toBeVisible();
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

test.describe("alerts: detail acknowledgment reactivity", () => {
  test("acknowledge hides ack button and shows status badge", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({ alert_number: 20, status: "firing", acknowledged: false });
    await mockAlertDetailApis(page, 20, { alert });

    await page.route("**/api/v1/alerts/20/acknowledge", (route) =>
      route.fulfill(dataEnvelope({ ...alert, acknowledged: true })),
    );

    await page.goto("/alerts/20");
    const ackBtn = page.getByRole("button", { name: /acknowledge/i });
    await expect(ackBtn).toBeVisible();
    await ackBtn.click();

    await expect(ackBtn).not.toBeVisible();
    await expect(page.getByText(/open/i).first()).toBeVisible();
  });
});

test.describe("alerts: detail resolve and reopen", () => {
  test("resolve updates status badge to RESOLVED", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({ alert_number: 21, status: "firing", acknowledged: true });
    await mockAlertDetailApis(page, 21, { alert });

    await page.route("**/api/v1/alerts/21/resolve", (route) =>
      route.fulfill(dataEnvelope({ ...alert, status: "resolved" })),
    );

    await page.goto("/alerts/21");
    await page.getByRole("button", { name: "Alert actions" }).click();
    await page.getByRole("menuitem", { name: "Mark resolved" }).click();

    await expect(page.getByText("RESOLVED").first()).toBeVisible();
  });

  test("reopen updates status badge to OPEN", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({ alert_number: 22, status: "resolved", acknowledged: true });
    await mockAlertDetailApis(page, 22, { alert });

    await page.route("**/api/v1/alerts/22/reopen", (route) =>
      route.fulfill(dataEnvelope({ ...alert, status: "firing" })),
    );

    await page.goto("/alerts/22");
    await page.getByRole("button", { name: "Alert actions" }).click();
    await page.getByRole("menuitem", { name: "Re-open" }).click();

    await expect(page.getByText("OPEN").first()).toBeVisible();
  });
});

test.describe("alerts: detail assignment", () => {
  test("assign to user via picker sends PATCH", async ({ page }) => {
    await mockAuthenticated(page);
    const investigation = makeInvestigation({ assignee_type: "agent", assignee_id: "agent-001" });
    const alert = makeAlert({ alert_number: 30 });
    await mockAlertDetailApis(page, 30, {
      alert,
      investigation,
      users: [makeUser({ id: "user-op-1", full_name: "Opie Operator" })],
    });

    let assignCalled = false;
    let assignBody: Record<string, unknown> = {};
    await page.route("**/api/v1/alert-investigations/ainv-001/assign", (route) => {
      assignCalled = true;
      assignBody = route.request().postDataJSON();
      return route.fulfill(
        dataEnvelope({ ...investigation, assignee_type: "user", assignee_id: "user-op-1" }),
      );
    });

    await page.goto("/alerts/30");
    await expect(page.getByText("Hermes").first()).toBeVisible();

    await page.getByRole("button", { name: "Reassign investigation" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByText("Opie Operator").click();

    await expect.poll(() => assignCalled).toBe(true);
    expect(assignBody.assignee_type).toBe("user");
    expect(assignBody.assignee_id).toBe("user-op-1");
  });

  test("reassign to agent pool sends PATCH with agent type", async ({ page }) => {
    await mockAuthenticated(page);
    const investigation = makeInvestigation({ assignee_type: "user", assignee_id: "user-op-1" });
    const alert = makeAlert({ alert_number: 31 });
    await mockAlertDetailApis(page, 31, {
      alert,
      investigation: { ...investigation, agent_name: "Opie Operator" },
      users: [makeUser()],
    });

    let assignCalled = false;
    let assignBody: Record<string, unknown> = {};
    await page.route("**/api/v1/alert-investigations/ainv-001/assign", (route) => {
      assignCalled = true;
      assignBody = route.request().postDataJSON();
      return route.fulfill(
        dataEnvelope({ ...investigation, assignee_type: "agent", assignee_id: "agent-001" }),
      );
    });

    await page.goto("/alerts/31");
    await page.getByRole("button", { name: "Reassign investigation" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByText("Agent (auto-dispatch)").click();

    await expect.poll(() => assignCalled).toBe(true);
    expect(assignBody.assignee_type).toBe("agent");
  });

  test("unassigned investigation shows assign button", async ({ page }) => {
    await mockAuthenticated(page);
    const investigation = makeInvestigation({
      agent_name: "",
      assignee_type: "agent",
      assignee_id: "",
    });
    const alert = makeAlert({ alert_number: 32 });
    await mockAlertDetailApis(page, 32, { alert, investigation });

    await page.goto("/alerts/32");
    await expect(page.getByRole("button", { name: /assign/i }).first()).toBeVisible();
  });
});

test.describe("alerts: detail reactivity", () => {
  test("shows loading skeleton then content", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({
      alert_number: 40,
      labels: { alertname: "SkeletonTest", severity: "warning" },
    });
    await mockAlertDetailApis(page, 40, { alert });

    await page.goto("/alerts/40");
    await expect(page.getByText("SkeletonTest").first()).toBeVisible();
    await expect(page.locator("[aria-busy='true']")).not.toBeVisible();
  });

  test("deleted alert shows read-only banner", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({
      alert_number: 41,
      deleted_at: "2025-06-02T00:00:00Z",
    });
    await mockAlertDetailApis(page, 41, { alert });

    await page.goto("/alerts/41");
    await expect(page.getByText(/deleted.*read-only/i)).toBeVisible();
  });

  test("shows SSE disconnected banner when stream is unavailable", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({ alert_number: 43 });
    await page.route(`**/api/v1/alerts/43/related`, (route) =>
      route.fulfill(dataEnvelope({ related_alerts: [], incident: null })),
    );
    await page.route(`**/api/v1/alerts/43/thread**`, (route) =>
      route.fulfill(dataEnvelope({ messages: [] })),
    );
    await page.route(`**/api/v1/alerts/43`, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope({ alert }));
    });
    await page.route("**/api/v1/users**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/agent-tokens**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/integrations**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/events**", (route) => route.abort());

    await page.goto("/alerts/43");
    await expect(page.getByText(/live updates paused/i)).toBeVisible();
  });

  test("severity badge displays correct label", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({
      alert_number: 44,
      labels: { alertname: "SevTest", severity: "critical" },
    });
    await mockAlertDetailApis(page, 44, { alert });

    await page.goto("/alerts/44");
    await expect(page.getByText("CRITICAL").first()).toBeVisible();
  });

  test("timeline renders alert events", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({
      alert_number: 45,
      events: [
        { type: "fired", timestamp: "2025-06-01T10:00:00Z", source: "grafana" },
        {
          type: "acked",
          timestamp: "2025-06-01T10:05:00Z",
          source: "user",
          actor_display_name: "Admin User",
        },
      ],
    });
    await mockAlertDetailApis(page, 45, { alert });

    await page.goto("/alerts/45");
    await expect(page.getByText("Fired").first()).toBeVisible();
    await expect(page.getByText("Acknowledged").first()).toBeVisible();
  });
});

test.describe("alerts: SSE live updates (list)", () => {
  test("alert_updated SSE updates existing row in place", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    const original = makeAlert({
      alert_number: 50,
      labels: { alertname: "BeforeUpdate", severity: "warning" },
    });
    await page.route("**/api/v1/alerts**", (route) => route.fulfill(dataEnvelope([original])));

    const updated = {
      ...original,
      labels: { alertname: "AfterUpdate", severity: "critical" },
    };

    let releaseStream!: () => void;
    const streamGate = new Promise<void>((resolve) => {
      releaseStream = resolve;
    });
    await page.route("**/api/v1/events**", async (route) => {
      await streamGate;
      try {
        await route.fulfill({
          contentType: "text/event-stream",
          body: `retry: 30000\nevent: alert_updated\ndata: ${JSON.stringify(updated)}\n\n`,
        });
      } catch {
        // aborted on navigation
      }
    });

    await page.goto("/alerts");
    await expect(page.getByText("BeforeUpdate")).toBeVisible();
    await page.evaluate(() => sessionStorage.setItem("sse_no_reload_marker", "1"));
    releaseStream();

    await expect(page.getByText("AfterUpdate")).toBeVisible();
    await expect
      .poll(() => page.evaluate(() => sessionStorage.getItem("sse_no_reload_marker")))
      .toBe("1");
  });

  test("alert_deleted SSE removes row from list", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));
    const alert = makeAlert({
      alert_number: 51,
      labels: { alertname: "ToDelete", severity: "info" },
    });
    await page.route("**/api/v1/alerts**", (route) => route.fulfill(dataEnvelope([alert])));

    let releaseStream!: () => void;
    const streamGate = new Promise<void>((resolve) => {
      releaseStream = resolve;
    });
    await page.route("**/api/v1/events**", async (route) => {
      await streamGate;
      try {
        await route.fulfill({
          contentType: "text/event-stream",
          body: `retry: 30000\nevent: alert_deleted\ndata: ${JSON.stringify(alert)}\n\n`,
        });
      } catch {
        // aborted on navigation
      }
    });

    await page.goto("/alerts");
    await expect(page.getByText("ToDelete")).toBeVisible();
    releaseStream();

    await expect(page.getByText("ToDelete")).not.toBeVisible();
  });
});

test.describe("alerts: SSE live updates (detail)", () => {
  test("alert_updated SSE triggers silent reload of detail", async ({ page }) => {
    await mockAuthenticated(page);
    const alert = makeAlert({
      alert_number: 60,
      labels: { alertname: "SSEReload", severity: "warning" },
    });

    let getRequestCount = 0;
    await page.route(`**/api/v1/alerts/60/related`, (route) =>
      route.fulfill(dataEnvelope({ related_alerts: [], incident: null })),
    );
    await page.route(`**/api/v1/alerts/60/thread**`, (route) =>
      route.fulfill(dataEnvelope({ messages: [] })),
    );
    await page.route(`**/api/v1/alerts/60`, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      getRequestCount++;
      return route.fulfill(dataEnvelope({ alert }));
    });
    await page.route("**/api/v1/users**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/agent-tokens**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/integrations**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/notifications**", (route) => route.fulfill(dataEnvelope([])));

    let releaseStream!: () => void;
    const streamGate = new Promise<void>((resolve) => {
      releaseStream = resolve;
    });
    await page.route("**/api/v1/events**", async (route) => {
      await streamGate;
      try {
        await route.fulfill({
          contentType: "text/event-stream",
          body: `retry: 30000\nevent: alert_updated\ndata: ${JSON.stringify(alert)}\n\n`,
        });
      } catch {
        // aborted on navigation
      }
    });

    await page.goto("/alerts/60");
    await expect(page.getByText("SSEReload").first()).toBeVisible();
    const countBefore = getRequestCount;
    releaseStream();

    await expect.poll(() => getRequestCount, { timeout: 10000 }).toBeGreaterThan(countBefore);
  });
});
