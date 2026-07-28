import { type Page, type Route } from "@playwright/test";

export type MockUser = {
  id: string;
  email: string;
  full_name: string;
  role: string;
  permissions: string[];
  created_at: string;
};

export const ADMIN_USER: MockUser = {
  id: "user-admin-1",
  email: "admin@alga.test",
  full_name: "Admin User",
  role: "admin",
  permissions: ["*"],
  created_at: "2025-01-01T00:00:00Z",
};

export const VIEWER_USER: MockUser = {
  id: "user-viewer-1",
  email: "viewer@alga.test",
  full_name: "Viewer User",
  role: "viewer",
  permissions: ["alerts:read", "incidents:read"],
  created_at: "2025-01-01T00:00:00Z",
};

export function json(body: unknown) {
  return { contentType: "application/json", body: JSON.stringify(body) };
}

export function dataEnvelope(payload: unknown) {
  return json({ data: payload });
}

export async function mockSetupStatus(page: Page, needsSetup: boolean) {
  await page.route("**/api/v1/setup/status", (route: Route) =>
    route.fulfill(json({ needs_setup: needsSetup })),
  );
}

export async function mockOnboardingStatus(page: Page, completed: boolean) {
  await page.route("**/api/v1/onboarding/status", (route: Route) =>
    route.fulfill(json({ completed })),
  );
}

export async function mockAuthMe(page: Page, user: MockUser | null) {
  await page.route("**/api/v1/auth/me", (route: Route) => {
    if (!user) {
      return route.fulfill({ status: 401, ...json({ error: "Unauthorized" }) });
    }
    return route.fulfill(dataEnvelope(user));
  });
}

export async function mockOAuthDisabled(page: Page) {
  await page.route("**/api/v1/auth/google/enabled", (route: Route) =>
    route.fulfill(json({ enabled: false })),
  );
  await page.route("**/api/v1/auth/slack/enabled", (route: Route) =>
    route.fulfill(json({ enabled: false })),
  );
  await page.route("**/api/v1/auth/oidc/providers", (route: Route) =>
    route.fulfill(dataEnvelope([])),
  );
}

export async function mockAuthenticated(page: Page, user: MockUser = ADMIN_USER) {
  await mockSetupStatus(page, false);
  await mockOnboardingStatus(page, true);
  await mockAuthMe(page, user);
  await mockOAuthDisabled(page);
}

export async function mockUnauthenticated(page: Page) {
  await mockSetupStatus(page, false);
  await mockAuthMe(page, null);
  await mockOAuthDisabled(page);
}

export function makeAlert(overrides: Record<string, unknown> = {}) {
  return {
    fingerprint: "fp-test-001",
    alert_number: 1,
    status: "firing",
    acknowledged: false,
    silenced: false,
    labels: { alertname: "HighCPU", severity: "critical" },
    annotations: { summary: "CPU usage above 90%" },
    values: null,
    starts_at: "2025-06-01T10:00:00Z",
    ends_at: null,
    generator_url: "",
    events: [],
    updated_at: "2025-06-01T10:00:00Z",
    created_at: "2025-06-01T10:00:00Z",
    deleted_at: null,
    ...overrides,
  };
}

export function makeIncident(overrides: Record<string, unknown> = {}) {
  return {
    id: "inc-001",
    incident_number: 1,
    title: "Database connection pool exhausted",
    description: "Primary DB connection pool at capacity",
    status: "active",
    severity: "critical",
    impact_level: "high",
    priority: "P1",
    incident_type: "infrastructure",
    slack_channel_archived: false,
    created_at: "2025-06-01T10:00:00Z",
    updated_at: "2025-06-01T10:00:00Z",
    deleted_at: null,
    ...overrides,
  };
}
