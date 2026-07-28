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

export function makeUser(overrides: Record<string, unknown> = {}) {
  return {
    id: "user-op-1",
    email: "operator@alga.test",
    full_name: "Opie Operator",
    role: "operator",
    permissions: ["alerts:read", "alerts:write", "incidents:read", "incidents:write"],
    created_at: "2025-01-02T00:00:00Z",
    ...overrides,
  };
}

export function makeNotification(overrides: Record<string, unknown> = {}) {
  return {
    id: "notif-001",
    user_id: "user-admin-1",
    type: "incident",
    title: "Incident #5 declared",
    message: "Payment API degraded",
    read: false,
    resource_type: "incident",
    resource_id: "5",
    created_at: "2025-06-01T10:05:00Z",
    ...overrides,
  };
}

export function makeSchedule(overrides: Record<string, unknown> = {}) {
  return {
    id: "sched-001",
    team_id: "team-001",
    team_name: "Platform Team",
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    layers: [],
    ...overrides,
  };
}

export function makeOnCallEntry(overrides: Record<string, unknown> = {}) {
  return {
    schedule_id: "sched-001",
    schedule_name: "Platform Team",
    user_id: "user-op-1",
    user_display_name: "Priya Sharma",
    until: new Date(Date.now() + 6 * 3600_000).toISOString(),
    ...overrides,
  };
}

export function makeDashboardStats(overrides: Record<string, unknown> = {}) {
  return {
    alerts: { total: 120, firing: 5, resolved: 115, unacknowledged: 3 },
    alerts_by_severity: [
      { severity: "critical", count: 2 },
      { severity: "warning", count: 3 },
    ],
    alert_trend: [],
    investigations: {
      total: 10,
      complete: 6,
      investigating: 1,
      pending: 1,
      failed: 1,
      cancelled: 1,
      timed_out: 0,
      completion_rate: 60,
    },
    top_alerts_24h: [],
    recent_investigations: [],
    active_investigations: [],
    incidents: {
      total: 30,
      active: 2,
      mitigated: 1,
      resolved: 27,
      by_severity: { critical: 4 },
      by_priority: { P1: 2, P2: 1 },
    },
    active_incidents: [
      {
        incident_number: 21,
        title: "Payment API latency",
        severity: "critical",
        priority: "P1",
        status: "active",
        service_name: "payments",
        created_at: "2025-06-01T09:00:00Z",
      },
    ],
    services: { total: 8, by_status: { operational: 8 } },
    sla_stats: { response_breaches: 0, resolve_breaches: 1, compliance_pct: 97.5 },
    ...overrides,
  };
}

export function makeIncidentMetrics(overrides: Record<string, unknown> = {}) {
  return {
    mtta_minutes: 12.5,
    mttr_minutes: 95,
    mttm_minutes: 40,
    total_created: 30,
    total_resolved: 27,
    by_severity: {},
    by_priority: {},
    by_service: {},
    sla_compliance: {
      response_sla_compliance_pct: 99,
      resolve_sla_compliance_pct: 97.5,
      response_breaches: 0,
      resolve_breaches: 1,
      total_with_sla: 40,
    },
    trend: [
      { date: "2025-05-30", created: 3, resolved: 2, mtta_minutes: 10, mttr_minutes: 80 },
      { date: "2025-05-31", created: 4, resolved: 4, mtta_minutes: 15, mttr_minutes: 110 },
    ],
    ...overrides,
  };
}

export async function mockDashboardApis(
  page: Page,
  options: {
    stats?: unknown;
    metrics?: unknown;
    onCall?: unknown;
    services?: unknown;
    summary?: unknown;
  } = {},
) {
  await page.route("**/api/v1/dashboard/stats", (route: Route) =>
    route.fulfill(dataEnvelope(options.stats ?? makeDashboardStats())),
  );
  await page.route("**/api/v1/dashboard/daily-summary**", (route: Route) =>
    route.fulfill(
      dataEnvelope(
        options.summary ?? {
          summary: "",
          generated_at: "",
          period: "",
          available: false,
        },
      ),
    ),
  );
  await page.route("**/api/v1/incidents/metrics**", (route: Route) =>
    route.fulfill(dataEnvelope(options.metrics ?? makeIncidentMetrics())),
  );
  await page.route("**/api/v1/on-call/**", (route: Route) =>
    route.fulfill(dataEnvelope(options.onCall ?? [])),
  );
  await page.route("**/api/v1/services**", (route: Route) =>
    route.fulfill(dataEnvelope(options.services ?? { items: [], total: 0 })),
  );
  await page.route("**/api/v1/notifications**", (route: Route) => route.fulfill(dataEnvelope([])));
  await page.route("**/api/v1/events**", (route: Route) => route.abort());
}
