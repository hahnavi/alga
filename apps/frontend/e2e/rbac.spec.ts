import { expect, test } from "@playwright/test";
import { ADMIN_USER, VIEWER_USER, mockAuthenticated } from "./helpers";

test.describe("rbac: permission-based redirects", () => {
  test("viewer without incidents:write can still view /incidents (has incidents:read)", async ({
    page,
  }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.route("**/api/v1/incidents**", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { items: [], total: 0 } }),
      }),
    );
    await page.goto("/incidents");
    await expect(page).toHaveURL(/\/incidents/);
  });

  test("viewer without routes:read is redirected from /routes", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.goto("/routes");
    await expect(page).toHaveURL("/");
  });

  test("viewer without tokens:manage is redirected from /agents", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.goto("/agents");
    await expect(page).toHaveURL("/");
  });

  test("viewer without users:manage is redirected from /users", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.goto("/users");
    await expect(page).toHaveURL("/");
  });

  test("viewer without system:read is redirected from /settings/authentication", async ({
    page,
  }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.goto("/settings/authentication");
    await expect(page).toHaveURL("/");
  });

  test("viewer without oncall:read is redirected from /on-call", async ({ page }) => {
    await mockAuthenticated(page, VIEWER_USER);
    await page.goto("/on-call");
    await expect(page).toHaveURL("/");
  });
});

test.describe("rbac: admin access", () => {
  test("admin with wildcard permission can access /routes", async ({ page }) => {
    await mockAuthenticated(page, ADMIN_USER);
    await page.route("**/api/v1/routes**", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: [] }),
      }),
    );
    await page.goto("/routes");
    await expect(page).toHaveURL(/\/routes/);
  });

  test("admin can access /users", async ({ page }) => {
    await mockAuthenticated(page, ADMIN_USER);
    await page.route("**/api/v1/users**", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: [ADMIN_USER] }),
      }),
    );
    await page.goto("/users");
    await expect(page).toHaveURL(/\/users/);
  });

  test("admin can access /system/general", async ({ page }) => {
    await mockAuthenticated(page, ADMIN_USER);
    await page.route("**/api/v1/system/**", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: {} }),
      }),
    );
    await page.goto("/system/general");
    await expect(page).toHaveURL(/\/system\/general/);
  });
});
