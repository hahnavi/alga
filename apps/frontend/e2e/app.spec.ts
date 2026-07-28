import { expect, test } from "@playwright/test";

test("app loads and renders", async ({ page }) => {
  await page.goto("/");
  // Both entry routes (/login "Welcome back", /setup "Welcome to Alga") render
  // this heading only after the Vue app mounts, unlike the static #app node.
  await expect(page.getByRole("heading", { name: /welcome/i })).toBeVisible();
});
