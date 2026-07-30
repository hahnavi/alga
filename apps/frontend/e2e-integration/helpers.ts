import type { Page } from "@playwright/test";

export const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL ?? "admin@alga-e2e.test";
export const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? "E2e!Str0ngPass1";
export const ADMIN_NAME = "E2E Admin";

export const VIEWER_EMAIL = process.env.E2E_VIEWER_EMAIL ?? "viewer@alga-e2e.test";
export const VIEWER_PASSWORD = process.env.E2E_VIEWER_PASSWORD ?? "Vi3wer!Pass123";
export const VIEWER_NAME = "E2E Viewer";

export async function getCsrfToken(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  const csrf = cookies.find((c) => c.name === "alga_csrf");
  return csrf?.value ?? "";
}
