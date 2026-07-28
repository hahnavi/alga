import { expect, test } from "@playwright/test";
import {
  ADMIN_USER,
  dataEnvelope,
  json,
  mockAuthenticated,
  mockOAuthDisabled,
  mockOnboardingStatus,
  mockSetupStatus,
  mockUnauthenticated,
} from "./helpers";

test.describe("auth: unauthenticated redirects", () => {
  test("navigating to / redirects to /login", async ({ page }) => {
    await mockUnauthenticated(page);
    await page.goto("/");
    await expect(page).toHaveURL(/\/login/);
    await expect(page.getByRole("heading", { name: /welcome back/i })).toBeVisible();
  });

  test("navigating to /alerts redirects to /login", async ({ page }) => {
    await mockUnauthenticated(page);
    await page.goto("/alerts");
    await expect(page).toHaveURL(/\/login/);
  });

  test("navigating to /incidents redirects to /login", async ({ page }) => {
    await mockUnauthenticated(page);
    await page.goto("/incidents");
    await expect(page).toHaveURL(/\/login/);
  });
});

test.describe("auth: login flow", () => {
  test("successful login navigates to dashboard", async ({ page }) => {
    await mockUnauthenticated(page);
    await page.route("**/api/v1/auth/login", (route) =>
      route.fulfill(dataEnvelope({ ...ADMIN_USER, csrf_token: "test-csrf" })),
    );
    await mockOnboardingStatus(page, true);

    await page.goto("/login");
    await page.fill("#login-email", "admin@alga.test");
    await page.fill("#login-password", "password123");
    await page.getByRole("button", { name: /sign in/i }).click();

    await expect(page).toHaveURL("/");
  });

  test("failed login shows error banner", async ({ page }) => {
    await mockUnauthenticated(page);
    await page.route("**/api/v1/auth/login", (route) =>
      route.fulfill({ status: 401, ...json({ error: "Invalid credentials" }) }),
    );

    await page.goto("/login");
    await page.fill("#login-email", "admin@alga.test");
    await page.fill("#login-password", "wrong");
    await page.getByRole("button", { name: /sign in/i }).click();

    await expect(page.getByText(/invalid credentials/i)).toBeVisible();
    await expect(page).toHaveURL(/\/login/);
  });

  test("blur on empty fields shows validation state", async ({ page }) => {
    await mockUnauthenticated(page);
    await page.goto("/login");
    await page.locator("#login-email").click();
    await page.keyboard.press("Tab");
    await page.locator("#login-password").click();
    await page.keyboard.press("Tab");

    await expect(page.locator("#login-email")).toHaveAttribute("aria-invalid", "true");
    await expect(page.getByText(/password is required/i)).toBeVisible();
  });
});

test.describe("auth: logout", () => {
  test("logout redirects to login", async ({ page }) => {
    await mockSetupStatus(page, false);
    await mockOnboardingStatus(page, true);
    await mockOAuthDisabled(page);
    await page.route("**/api/v1/auth/me", (route) => route.fulfill(dataEnvelope(ADMIN_USER)));
    await page.route("**/api/v1/notifications/**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/dashboard/**", (route) => route.fulfill(dataEnvelope({})));
    await page.route("**/api/v1/events**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/incidents/metrics**", (route) => route.fulfill(dataEnvelope({})));
    await page.route("**/api/v1/on-call/**", (route) => route.fulfill(dataEnvelope([])));
    await page.route("**/api/v1/services**", (route) =>
      route.fulfill(dataEnvelope({ items: [], total: 0 })),
    );

    let loggedOut = false;
    await page.route("**/api/v1/auth/logout", (route) => {
      loggedOut = true;
      return route.fulfill(json({ status: "ok" }));
    });

    await page.goto("/");
    await expect(page).toHaveURL("/");

    await page.getByLabel(/account menu/i).click();
    await page.getByRole("menuitem", { name: /logout/i }).click();
    await expect.poll(() => loggedOut).toBe(true);

    await page.unroute("**/api/v1/auth/me");
    await page.route("**/api/v1/auth/me", (route) =>
      route.fulfill({ status: 401, ...json({ error: "Unauthorized" }) }),
    );
    await page.reload();
    await expect(page).toHaveURL(/\/login/);
  });
});

test.describe("auth: setup wizard", () => {
  test("setup required redirects all traffic to /setup", async ({ page }) => {
    await mockSetupStatus(page, true);
    await page.goto("/alerts");
    await expect(page).toHaveURL(/\/setup/);
    await expect(page.getByRole("heading", { name: /welcome to alga/i })).toBeVisible();
  });

  test("setup complete redirects /setup to /login", async ({ page }) => {
    await mockSetupStatus(page, false);
    await page.route("**/api/v1/auth/me", (route) =>
      route.fulfill({ status: 401, ...json({ error: "Unauthorized" }) }),
    );
    await mockOAuthDisabled(page);

    await page.goto("/setup");
    await expect(page).toHaveURL(/\/login/);
  });
});

test.describe("auth: onboarding gate", () => {
  test("incomplete onboarding redirects to /onboarding", async ({ page }) => {
    await mockSetupStatus(page, false);
    await page.route("**/api/v1/auth/me", (route) => route.fulfill(dataEnvelope(ADMIN_USER)));
    await mockOnboardingStatus(page, false);
    await mockOAuthDisabled(page);

    await page.goto("/alerts");
    await expect(page).toHaveURL(/\/onboarding/);
  });
});

test.describe("auth: 404 page", () => {
  test("unknown route shows not found page", async ({ page }) => {
    await mockAuthenticated(page);
    await page.route("**/api/v1/**", (route) => {
      if (route.request().url().includes("/auth/me")) {
        return route.fulfill(dataEnvelope(ADMIN_USER));
      }
      if (route.request().url().includes("/setup/status")) {
        return route.fulfill(json({ needs_setup: false }));
      }
      if (route.request().url().includes("/onboarding/status")) {
        return route.fulfill(json({ completed: true }));
      }
      return route.fulfill(dataEnvelope([]));
    });

    await page.goto("/this-does-not-exist");
    await expect(page.getByText("404")).toBeVisible();
    await expect(page.getByText(/page not found/i)).toBeVisible();
  });
});
