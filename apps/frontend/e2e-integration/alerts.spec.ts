import { expect, test } from "@playwright/test";

async function getCsrfToken(page: any): Promise<string> {
  const cookies = await page.context().cookies();
  const csrf = cookies.find((c: { name: string }) => c.name === "alga_csrf");
  return csrf?.value ?? "";
}

async function createWebhookToken(page: any): Promise<string> {
  const csrf = await getCsrfToken(page);
  const res = await page.request.post("/api/v1/webhook-tokens", {
    data: { name: `e2e-${Date.now()}` },
    headers: { "X-CSRF-Token": csrf },
  });
  expect(res.status()).toBe(201);
  const body = await res.json();
  return body.data.token;
}

function grafanaPayload(fingerprint: string, alertname: string, severity: string) {
  const now = new Date().toISOString();
  return {
    receiver: "e2e",
    status: "firing",
    orgId: 1,
    alerts: [
      {
        status: "firing",
        labels: { alertname, severity, service: "e2e-test" },
        annotations: { summary: `E2E alert: ${alertname}` },
        startsAt: now,
        endsAt: "0001-01-01T00:00:00Z",
        generatorURL: "http://grafana:3000/alerting/1/edit",
        fingerprint,
        values: {},
        valueString: "",
      },
    ],
    groupLabels: { alertname },
    commonLabels: { alertname, severity },
    commonAnnotations: { summary: `E2E alert: ${alertname}` },
    externalURL: "http://grafana:3000",
    version: "4",
    groupKey: `e2e:${fingerprint}`,
    truncatedAlerts: 0,
    title: `[FIRING] ${alertname}`,
    state: "alerting",
    message: `E2E alert: ${alertname}`,
  };
}

test.describe("alerts: webhook ingestion", () => {
  test("ingested alert appears in the alerts list", async ({ page }) => {
    const token = await createWebhookToken(page);
    const fingerprint = `e2e-fp-${Date.now()}`;
    const alertname = `E2EAlert${Date.now()}`;

    const ingestRes = await page.request.post("/webhooks/alerts", {
      data: grafanaPayload(fingerprint, alertname, "critical"),
      headers: { Authorization: `Bearer ${token}` },
    });
    expect([200, 202]).toContain(ingestRes.status());

    await page.goto("/alerts");
    await expect(page.getByText(alertname).first()).toBeVisible({ timeout: 15_000 });
  });

  test("webhook rejects invalid token", async ({ page }) => {
    const res = await page.request.post("/webhooks/alerts", {
      data: grafanaPayload("fp-bad", "BadAlert", "warning"),
      headers: { Authorization: "Bearer alga_invalid_token_here" },
    });
    expect(res.status()).toBe(401);
  });

  test("webhook rejects missing token", async ({ page }) => {
    const res = await page.request.post("/webhooks/alerts", {
      data: grafanaPayload("fp-none", "NoToken", "info"),
    });
    expect(res.status()).toBe(401);
  });

  test("resolved alert updates status in UI", async ({ page }) => {
    const token = await createWebhookToken(page);
    const fingerprint = `e2e-resolve-${Date.now()}`;
    const alertname = `E2EResolve${Date.now()}`;

    const fireRes = await page.request.post("/webhooks/alerts", {
      data: grafanaPayload(fingerprint, alertname, "warning"),
      headers: { Authorization: `Bearer ${token}` },
    });
    expect([200, 202]).toContain(fireRes.status());

    await page.goto("/alerts");
    await expect(page.getByText(alertname).first()).toBeVisible({ timeout: 15_000 });

    const now = new Date().toISOString();
    const resolvePayload = grafanaPayload(fingerprint, alertname, "warning");
    resolvePayload.status = "resolved";
    resolvePayload.state = "ok";
    resolvePayload.alerts[0].status = "resolved";
    resolvePayload.alerts[0].endsAt = now;

    const resolveRes = await page.request.post("/webhooks/alerts", {
      data: resolvePayload,
      headers: { Authorization: `Bearer ${token}` },
    });
    expect([200, 202]).toContain(resolveRes.status());

    await page.reload();
    await expect(page.getByText(/resolved/i).first()).toBeVisible({ timeout: 15_000 });
  });
});

test.describe("alerts: detail view", () => {
  test("clicking an alert shows its labels and annotations", async ({ page }) => {
    const token = await createWebhookToken(page);
    const fingerprint = `e2e-detail-${Date.now()}`;
    const alertname = `E2EDetail${Date.now()}`;

    await page.request.post("/webhooks/alerts", {
      data: grafanaPayload(fingerprint, alertname, "critical"),
      headers: { Authorization: `Bearer ${token}` },
    });

    await page.goto("/alerts");
    await expect(page.getByText(alertname).first()).toBeVisible({ timeout: 15_000 });
    await page.getByText(alertname).first().click();

    await expect(page.getByText(/critical/i).first()).toBeVisible();
  });
});
