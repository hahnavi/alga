import { defineConfig, devices } from "@playwright/test";

// E2E_INTEGRATION=1 runs the full-stack suite (e2e-integration/) against a
// real backend; otherwise the mocked suite (e2e/) runs against the Vite dev
// server. One config because webServer and workers are global-only options.
const integration = !!process.env.E2E_INTEGRATION;
const integrationBaseURL = process.env.E2E_BASE_URL ?? "http://localhost:3100";

export default defineConfig({
  fullyParallel: !integration,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? (integration ? 1 : 2) : 0,
  workers: integration || process.env.CI ? 1 : undefined,
  reporter: "html",
  timeout: 30_000,
  use: {
    baseURL: integration ? integrationBaseURL : "http://localhost:5173",
    trace: "on-first-retry",
  },
  projects: integration
    ? [
        {
          name: "setup",
          testDir: "./e2e-integration",
          testMatch: /global\.setup\.ts/,
          // Must exceed the 60s backend readiness poll in global.setup.ts.
          timeout: 120_000,
        },
        {
          name: "unauthenticated",
          testDir: "./e2e-integration",
          use: { ...devices["Desktop Chrome"] },
          dependencies: ["setup"],
          testMatch: /auth\.spec\.ts/,
        },
        {
          name: "integration",
          testDir: "./e2e-integration",
          use: {
            ...devices["Desktop Chrome"],
            storageState: "e2e-integration/.auth/admin.json",
          },
          dependencies: ["setup"],
          testIgnore: [/global\.setup\.ts/, /auth\.spec\.ts/],
        },
      ]
    : [
        {
          name: "chromium",
          testDir: "./e2e",
          use: { ...devices["Desktop Chrome"] },
        },
      ],
  webServer: integration
    ? undefined
    : {
        command: "pnpm dev",
        url: "http://localhost:5173",
        reuseExistingServer: !process.env.CI,
      },
});
