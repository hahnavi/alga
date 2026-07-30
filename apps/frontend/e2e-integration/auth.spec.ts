import { expect, test } from "@playwright/test";

const ADMIN_EMAIL = "admin@alga-e2e.test";
const ADMIN_PASSWORD = "E2e!Str0ngPass1";

test.describe("auth: login flow", () => {
  test("setup wizard is no longer accessible after admin creation", async ({ page }) => {
    await page.goto("/setup");
    await expect(page).not.toHaveURL(/\/setup/);
  });

  test("unauthenticated user is redirected to login", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/login/);
  });

  test("login with valid credentials reaches the dashboard", async ({ page }) => {
    await page.goto("/login");
    await page.fill("#login-email", ADMIN_EMAIL);
    await page.fill("#login-password", ADMIN_PASSWORD);
    await page.locator('button[type="submit"]').click();

    await expect(page).toHaveURL("/", { timeout: 10_000 });
    await expect(page.getByText(/dashboard|overview/i).first()).toBeVisible();
  });

  test("login with wrong password shows error", async ({ page }) => {
    await page.goto("/login");
    await page.fill("#login-email", ADMIN_EMAIL);
    await page.fill("#login-password", "Wrong!Pass123");
    await page.locator('button[type="submit"]').click();

    await expect(page.getByText(/invalid|incorrect|failed/i)).toBeVisible();
    await expect(page).toHaveURL(/\/login/);
  });

  test("logout returns to login page", async ({ page }) => {
    await page.goto("/login");
    await page.fill("#login-email", ADMIN_EMAIL);
    await page.fill("#login-password", ADMIN_PASSWORD);
    await page.locator('button[type="submit"]').click();
    await expect(page).toHaveURL("/", { timeout: 10_000 });

    const cookies = await page.context().cookies();
    const csrfCookie = cookies.find((c) => c.name === "alga_csrf");
    expect(csrfCookie).toBeDefined();

    await page.request.post("/api/v1/auth/logout", {
      headers: { "X-CSRF-Token": csrfCookie!.value },
    });

    await page.goto("/");
    await expect(page).toHaveURL(/\/login/);
  });
});
