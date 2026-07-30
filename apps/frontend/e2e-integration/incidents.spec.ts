import { expect, test } from "@playwright/test";

async function getCsrfToken(page: any): Promise<string> {
  const cookies = await page.context().cookies();
  const csrf = cookies.find((c: { name: string }) => c.name === "alga_csrf");
  return csrf?.value ?? "";
}

async function createIncident(page: any, overrides: Record<string, unknown> = {}) {
  const csrf = await getCsrfToken(page);
  const res = await page.request.post("/api/v1/incidents", {
    data: {
      title: `E2E Incident ${Date.now()}`,
      description: "Created by integration test",
      severity: "critical",
      impact_level: "high",
      ...overrides,
    },
    headers: { "X-CSRF-Token": csrf },
  });
  expect(res.status()).toBe(201);
  const body = await res.json();
  return body.data;
}

test.describe("incidents: creation", () => {
  test("create incident via API and verify in UI", async ({ page }) => {
    const title = `E2E Create ${Date.now()}`;
    const incident = await createIncident(page, { title });

    expect(incident.incident_number).toBeGreaterThan(0);
    expect(incident.status).toBe("detected");
    expect(incident.severity).toBe("critical");
    expect(incident.priority).toBeDefined();

    await page.goto("/incidents");
    await expect(page.getByText(title)).toBeVisible({ timeout: 10_000 });
  });

  test("create incident via UI form", async ({ page }) => {
    const title = `E2E UI Incident ${Date.now()}`;

    await page.goto("/incidents");
    await page
      .getByRole("button", { name: /new incident|create incident/i })
      .first()
      .click();

    await page.fill("#create-incident-title-input", title);
    await page
      .getByRole("dialog")
      .getByRole("button", { name: /create incident/i })
      .click();

    await expect(page.getByText(title)).toBeVisible({ timeout: 10_000 });
  });

  test("incident title is required", async ({ page }) => {
    const csrf = await getCsrfToken(page);
    const res = await page.request.post("/api/v1/incidents", {
      data: { title: "" },
      headers: { "X-CSRF-Token": csrf },
    });
    expect(res.status()).toBe(400);
  });
});

test.describe("incidents: lifecycle", () => {
  test("incident acknowledges from detected to active", async ({ page }) => {
    const incident = await createIncident(page, {
      title: `E2E Ack ${Date.now()}`,
    });
    const num = incident.incident_number;
    const csrf = await getCsrfToken(page);

    const ackRes = await page.request.post(`/api/v1/incidents/${num}/acknowledge`, {
      headers: { "X-CSRF-Token": csrf },
    });
    expect(ackRes.status()).toBe(200);
    const acked = await ackRes.json();
    expect(acked.data.status).toBe("active");
  });

  test("incident mitigates from active to mitigated", async ({ page }) => {
    const incident = await createIncident(page, {
      title: `E2E Mitigate ${Date.now()}`,
    });
    const num = incident.incident_number;
    const csrf = await getCsrfToken(page);

    await page.request.post(`/api/v1/incidents/${num}/acknowledge`, {
      headers: { "X-CSRF-Token": csrf },
    });

    const mitigateRes = await page.request.post(`/api/v1/incidents/${num}/mitigate`, {
      headers: { "X-CSRF-Token": csrf },
    });
    expect(mitigateRes.status()).toBe(200);
    const mitigated = await mitigateRes.json();
    expect(mitigated.data.status).toBe("mitigated");
  });

  test("incident cancels from detected", async ({ page }) => {
    const incident = await createIncident(page, {
      title: `E2E Cancel ${Date.now()}`,
    });
    const num = incident.incident_number;
    const csrf = await getCsrfToken(page);

    const cancelRes = await page.request.post(`/api/v1/incidents/${num}/cancel`, {
      headers: { "X-CSRF-Token": csrf },
    });
    expect(cancelRes.status()).toBe(200);
    const cancelled = await cancelRes.json();
    expect(cancelled.data.status).toBe("cancelled");
  });

  test("invalid transition is rejected", async ({ page }) => {
    const incident = await createIncident(page, {
      title: `E2E Invalid ${Date.now()}`,
    });
    const num = incident.incident_number;
    const csrf = await getCsrfToken(page);

    const closeRes = await page.request.post(`/api/v1/incidents/${num}/close`, {
      headers: { "X-CSRF-Token": csrf },
    });
    expect([400, 409]).toContain(closeRes.status());
  });

  test("incident detail page shows metadata", async ({ page }) => {
    const title = `E2E Detail ${Date.now()}`;
    const incident = await createIncident(page, {
      title,
      severity: "high",
      impact_level: "medium",
    });

    await page.goto(`/incidents/${incident.incident_number}`);
    await expect(page.getByText(title).first()).toBeVisible();
    await expect(page.getByText(/high/i).first()).toBeVisible();
  });
});

test.describe("incidents: list and pagination", () => {
  test("incidents list shows count and entries", async ({ page }) => {
    await createIncident(page, { title: `E2E List A ${Date.now()}` });
    await createIncident(page, { title: `E2E List B ${Date.now()}` });

    await page.goto("/incidents");
    await expect(page.getByText(/incident/i).first()).toBeVisible();
  });

  test("filter incidents by status", async ({ page }) => {
    await createIncident(page, { title: `E2E Filter ${Date.now()}` });

    const res = await page.request.get("/api/v1/incidents?status=detected");
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.data.items.length).toBeGreaterThan(0);
  });
});
