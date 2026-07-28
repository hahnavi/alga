import { expect, test } from "@playwright/test";
import { ADMIN_USER, dataEnvelope, json, mockDashboardApis, mockSetupStatus } from "./helpers";

const STRONG_PASSWORD = "Str0ng!Passw0rd";

test.describe("setup: admin account creation", () => {
  test("completing the wizard POSTs setup and enters the app", async ({ page }) => {
    await mockSetupStatus(page, true);

    let setupDone = false;
    let setupBody: Record<string, unknown> | null = null;
    await page.route("**/api/v1/setup", (route) => {
      if (route.request().method() === "POST") {
        setupDone = true;
        setupBody = JSON.parse(route.request().postData() ?? "{}");
        return route.fulfill(dataEnvelope({ csrf_token: "test-csrf" }));
      }
      return route.fulfill({ status: 405, ...json({ error: "Method not allowed" }) });
    });
    // /setup is guest-only: before the admin exists, auth/me must 401; after
    // the setup POST it returns the freshly created admin session.
    await page.route("**/api/v1/auth/me", (route) => {
      if (!setupDone) {
        return route.fulfill({ status: 401, ...json({ error: "Unauthorized" }) });
      }
      return route.fulfill(dataEnvelope(ADMIN_USER));
    });
    await page.route("**/api/v1/onboarding/status", (route) =>
      route.fulfill(json({ completed: true })),
    );
    await page.route("**/api/v1/auth/google/enabled", (route) =>
      route.fulfill(json({ enabled: false })),
    );
    await page.route("**/api/v1/auth/slack/enabled", (route) =>
      route.fulfill(json({ enabled: false })),
    );
    await page.route("**/api/v1/auth/oidc/providers", (route) => route.fulfill(dataEnvelope([])));
    await mockDashboardApis(page);

    await page.goto("/setup");
    await expect(page.getByRole("heading", { name: /welcome to alga/i })).toBeVisible();

    await page.fill("#setup-full-name", "Admin User");
    await page.fill("#setup-email", "admin@alga.test");
    await page.fill("#setup-password", STRONG_PASSWORD);
    await page.fill("#setup-confirm-password", STRONG_PASSWORD);
    await page.getByRole("button", { name: /create admin account/i }).click();

    await expect(page).toHaveURL("/");
    expect(setupBody).toMatchObject({
      email: "admin@alga.test",
      full_name: "Admin User",
      password: STRONG_PASSWORD,
    });
  });

  test("setup API failure shows error banner and stays on /setup", async ({ page }) => {
    await mockSetupStatus(page, true);
    await page.route("**/api/v1/setup", (route) =>
      route.fulfill({ status: 409, ...json({ error: "Setup already completed" }) }),
    );

    await page.goto("/setup");
    await page.fill("#setup-full-name", "Admin User");
    await page.fill("#setup-email", "admin@alga.test");
    await page.fill("#setup-password", STRONG_PASSWORD);
    await page.fill("#setup-confirm-password", STRONG_PASSWORD);
    await page.getByRole("button", { name: /create admin account/i }).click();

    await expect(page.getByText(/setup already completed/i)).toBeVisible();
    await expect(page).toHaveURL(/\/setup/);
  });
});

test.describe("setup: validation", () => {
  test("weak password shows policy error and disables submit", async ({ page }) => {
    await mockSetupStatus(page, true);
    await page.goto("/setup");

    await page.fill("#setup-full-name", "Admin User");
    await page.fill("#setup-email", "admin@alga.test");
    await page.fill("#setup-password", "weakpass");
    await page.locator("#setup-password").blur();

    await expect(page.getByText(/one uppercase letter required/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /create admin account/i })).toBeDisabled();
  });

  test("mismatched confirmation shows error and disables submit", async ({ page }) => {
    await mockSetupStatus(page, true);
    await page.goto("/setup");

    await page.fill("#setup-password", STRONG_PASSWORD);
    await page.fill("#setup-confirm-password", "Different!1pass");
    await page.locator("#setup-confirm-password").blur();

    await expect(page.getByText(/passwords do not match/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /create admin account/i })).toBeDisabled();
  });

  test("invalid email shows error on blur", async ({ page }) => {
    await mockSetupStatus(page, true);
    await page.goto("/setup");

    await page.fill("#setup-email", "not-an-email");
    await page.locator("#setup-email").blur();

    await expect(page.locator("#setup-email")).toHaveAttribute("aria-invalid", "true");
    await expect(page.getByRole("button", { name: /create admin account/i })).toBeDisabled();
  });
});
