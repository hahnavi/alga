import { expect, test } from "@playwright/test";

const ADMIN_NAME = "E2E Admin";
const VIEWER_NAME = "E2E Viewer";

async function getCsrfToken(page: any): Promise<string> {
  const cookies = await page.context().cookies();
  const csrf = cookies.find((c: { name: string }) => c.name === "alga_csrf");
  return csrf?.value ?? "";
}

async function createManualAlert(page: any, alertname: string): Promise<number> {
  const csrf = await getCsrfToken(page);
  const res = await page.request.post("/api/v1/alerts", {
    data: { alertname, severity: "critical", message: `E2E manual alert: ${alertname}` },
    headers: { "X-CSRF-Token": csrf },
  });
  expect(res.status()).toBe(201);
  const body = await res.json();
  return body.data.alert_number;
}

async function ensureInvestigation(page: any, alertNumber: number): Promise<void> {
  const csrf = await getCsrfToken(page);
  const invRes = await page.request.post(`/api/v1/alerts/${alertNumber}/investigate`, {
    headers: { "X-CSRF-Token": csrf },
  });
  expect([200, 409]).toContain(invRes.status());

  await expect
    .poll(
      async () => {
        const res = await page.request.get(`/api/v1/alerts/${alertNumber}`);
        if (res.status() !== 200) return "";
        const body = await res.json();
        return body.data.alert_investigation?.alert_investigation_id ?? "";
      },
      { timeout: 30_000, intervals: [1_000] },
    )
    .not.toBe("");
}

test.describe("alerts: manual acknowledge", () => {
  test("acknowledge button records the acting user in the timeline", async ({ page }) => {
    const alertname = `E2EAck${Date.now()}`;
    const alertNumber = await createManualAlert(page, alertname);

    await page.goto(`/alerts/${alertNumber}`);
    const ackButton = page.getByRole("button", { name: /^acknowledge$/i });
    await expect(ackButton).toBeVisible({ timeout: 15_000 });
    await ackButton.click();

    const ackLabel = page.getByText("Acknowledged", { exact: true });
    await expect(ackLabel).toBeVisible({ timeout: 10_000 });
    const ackEntry = ackLabel.locator("..");
    await expect(ackEntry.getByText(`by ${ADMIN_NAME}`)).toBeVisible();
    await expect(page.getByRole("button", { name: /^acknowledge$/i })).toHaveCount(0);
  });

  test("acknowledge persists across reload", async ({ page }) => {
    const alertname = `E2EAckPersist${Date.now()}`;
    const alertNumber = await createManualAlert(page, alertname);

    await page.goto(`/alerts/${alertNumber}`);
    const ackButton = page.getByRole("button", { name: /^acknowledge$/i });
    await expect(ackButton).toBeVisible({ timeout: 15_000 });
    await ackButton.click();
    await expect(page.getByText("Acknowledged", { exact: true })).toBeVisible({
      timeout: 10_000,
    });

    await page.reload();
    const ackLabel = page.getByText("Acknowledged", { exact: true });
    await expect(ackLabel).toBeVisible({ timeout: 15_000 });
    const ackEntry = ackLabel.locator("..");
    await expect(ackEntry.getByText(`by ${ADMIN_NAME}`)).toBeVisible();
  });
});

test.describe("alerts: manual assignment", () => {
  test("assigning to a human user displays the assignee in the sidebar", async ({ page }) => {
    const alertname = `E2EAssign${Date.now()}`;
    const alertNumber = await createManualAlert(page, alertname);

    await ensureInvestigation(page, alertNumber);

    await page.goto(`/alerts/${alertNumber}`);
    const assignButton = page.getByRole("button", { name: /^assign$/i });
    await expect(assignButton).toBeVisible({ timeout: 15_000 });
    await assignButton.click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: new RegExp(VIEWER_NAME) }).click();

    const sidebar = page.locator("aside");
    await expect(sidebar.getByText(VIEWER_NAME)).toBeVisible({ timeout: 10_000 });
    await expect(sidebar.getByText("User", { exact: true })).toBeVisible();
    await expect(sidebar.getByText("Unassigned")).toHaveCount(0);
  });

  test("assignment persists across reload", async ({ page }) => {
    const alertname = `E2EAssignPersist${Date.now()}`;
    const alertNumber = await createManualAlert(page, alertname);

    await ensureInvestigation(page, alertNumber);

    await page.goto(`/alerts/${alertNumber}`);
    const assignButton = page.getByRole("button", { name: /^assign$/i });
    await expect(assignButton).toBeVisible({ timeout: 15_000 });
    await assignButton.click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: new RegExp(VIEWER_NAME) })
      .click();

    const sidebar = page.locator("aside");
    await expect(sidebar.getByText(VIEWER_NAME)).toBeVisible({ timeout: 10_000 });

    await page.reload();
    await expect(sidebar.getByText(VIEWER_NAME)).toBeVisible({ timeout: 15_000 });
    await expect(sidebar.getByText("User", { exact: true })).toBeVisible();
  });
});
