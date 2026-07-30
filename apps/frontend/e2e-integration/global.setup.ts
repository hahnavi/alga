import { expect, test as setup } from "@playwright/test";
import {
  ADMIN_EMAIL,
  ADMIN_NAME,
  ADMIN_PASSWORD,
  getCsrfToken,
  VIEWER_EMAIL,
  VIEWER_NAME,
  VIEWER_PASSWORD,
} from "./helpers";

setup("complete setup wizard and authenticate", async ({ page }) => {
  const baseURL = process.env.E2E_BASE_URL ?? "http://localhost:3100";

  await expect
    .poll(
      async () => {
        try {
          const res = await page.request.get(`${baseURL}/api/v1/setup/status`);
          return res.status();
        } catch {
          return 0;
        }
      },
      { timeout: 60_000, intervals: [2_000] },
    )
    .toBe(200);

  const statusRes = await page.request.get(`${baseURL}/api/v1/setup/status`);
  const status = await statusRes.json();

  if (status.needs_setup) {
    const setupRes = await page.request.post(`${baseURL}/api/v1/setup`, {
      data: { email: ADMIN_EMAIL, password: ADMIN_PASSWORD, full_name: ADMIN_NAME },
    });
    expect(setupRes.status()).toBe(200);
    const setupBody = await setupRes.json();
    expect(setupBody.email).toBe(ADMIN_EMAIL);
    expect(setupBody.role).toBe("admin");
  } else {
    // Retry or reused stack: setup already ran, log in instead.
    const loginRes = await page.request.post(`${baseURL}/api/v1/auth/login`, {
      data: { email: ADMIN_EMAIL, password: ADMIN_PASSWORD },
    });
    expect(loginRes.status()).toBe(200);
  }

  const csrf = await getCsrfToken(page);
  expect(csrf).toBeTruthy();

  const onboardingRes = await page.request.post(`${baseURL}/api/v1/onboarding/complete`, {
    headers: { "X-CSRF-Token": csrf },
  });
  expect(onboardingRes.status()).toBe(200);

  const usersRes = await page.request.get(`${baseURL}/api/v1/users`);
  expect(usersRes.status()).toBe(200);
  const usersBody = await usersRes.json();
  const viewerExists = usersBody.data.some((u: { email: string }) => u.email === VIEWER_EMAIL);

  if (!viewerExists) {
    const viewerRes = await page.request.post(`${baseURL}/api/v1/users`, {
      data: {
        email: VIEWER_EMAIL,
        password: VIEWER_PASSWORD,
        full_name: VIEWER_NAME,
        role: "viewer",
      },
      headers: { "X-CSRF-Token": csrf },
    });
    expect(viewerRes.status()).toBe(201);
  }

  await page.goto("/");
  await expect(page).not.toHaveURL(/\/(login|setup|onboarding)/);

  await page.context().storageState({ path: "e2e-integration/.auth/admin.json" });
});

setup("authenticate as viewer", async ({ browser }) => {
  const baseURL = process.env.E2E_BASE_URL ?? "http://localhost:3100";
  const context = await browser.newContext();
  const page = await context.newPage();

  await page.goto(`${baseURL}/login`);
  await page.fill("#login-email", VIEWER_EMAIL);
  await page.fill("#login-password", VIEWER_PASSWORD);
  await page.locator('button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 });

  await context.storageState({ path: "e2e-integration/.auth/viewer.json" });
  await context.close();
});
