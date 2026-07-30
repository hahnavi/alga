import { expect, test } from "@playwright/test";

async function getCsrfToken(page: any): Promise<string> {
  const cookies = await page.context().cookies();
  const csrf = cookies.find((c: { name: string }) => c.name === "alga_csrf");
  return csrf?.value ?? "";
}

test.describe("rbac: viewer restrictions", () => {
  test.use({ storageState: "e2e-integration/.auth/viewer.json" });

  test("viewer can read alerts but cannot create webhook tokens", async ({ page }) => {
    const csrf = await getCsrfToken(page);

    const alertsRes = await page.request.get("/api/v1/alerts");
    expect(alertsRes.status()).toBe(200);

    const tokenRes = await page.request.post("/api/v1/webhook-tokens", {
      data: { name: "viewer-attempt" },
      headers: { "X-CSRF-Token": csrf },
    });
    expect(tokenRes.status()).toBe(403);
  });

  test("viewer can read incidents but cannot create them", async ({ page }) => {
    const csrf = await getCsrfToken(page);

    const listRes = await page.request.get("/api/v1/incidents");
    expect(listRes.status()).toBe(200);

    const createRes = await page.request.post("/api/v1/incidents", {
      data: { title: "Viewer should not create" },
      headers: { "X-CSRF-Token": csrf },
    });
    expect(createRes.status()).toBe(403);
  });

  test("viewer cannot manage users", async ({ page }) => {
    const usersRes = await page.request.get("/api/v1/users");
    expect(usersRes.status()).toBe(403);
  });
});

test.describe("rbac: admin capabilities", () => {
  test("admin can manage users", async ({ page }) => {
    const listRes = await page.request.get("/api/v1/users");
    expect(listRes.status()).toBe(200);
    const body = await listRes.json();
    expect(body.data.length).toBeGreaterThanOrEqual(1);
  });

  test("admin can create and revoke webhook tokens", async ({ page }) => {
    const csrf = await getCsrfToken(page);

    const createRes = await page.request.post("/api/v1/webhook-tokens", {
      data: { name: `rbac-test-${Date.now()}` },
      headers: { "X-CSRF-Token": csrf },
    });
    expect(createRes.status()).toBe(201);
    const token = await createRes.json();
    expect(token.data.token).toMatch(/^alga_/);

    const revokeRes = await page.request.delete(`/api/v1/webhook-tokens/${token.data.id}`, {
      headers: { "X-CSRF-Token": csrf },
    });
    expect(revokeRes.status()).toBe(200);
  });
});

test.describe("rbac: unauthenticated access", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("API rejects requests without session", async ({ page }) => {
    const alertsRes = await page.request.get("/api/v1/alerts");
    expect(alertsRes.status()).toBe(401);

    const incidentsRes = await page.request.get("/api/v1/incidents");
    expect(incidentsRes.status()).toBe(401);

    const usersRes = await page.request.get("/api/v1/users");
    expect(usersRes.status()).toBe(401);
  });
});
