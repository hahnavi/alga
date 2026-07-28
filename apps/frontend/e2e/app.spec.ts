import { expect, test } from "@playwright/test";

test("app loads and renders", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator("#app")).toBeVisible();
});
