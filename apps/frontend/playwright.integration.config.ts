import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e-integration",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: "html",
  timeout: 30_000,
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3100",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "setup",
      testMatch: /global\.setup\.ts/,
    },
    {
      name: "unauthenticated",
      use: { ...devices["Desktop Chrome"] },
      dependencies: ["setup"],
      testMatch: /auth\.spec\.ts/,
    },
    {
      name: "integration",
      use: {
        ...devices["Desktop Chrome"],
        storageState: "e2e-integration/.auth/admin.json",
      },
      dependencies: ["setup"],
      testIgnore: [/global\.setup\.ts/, /auth\.spec\.ts/],
    },
  ],
});
