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
  await page.route("**/api/v1/setup/status", (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(json({ needs_setup: needsSetup }));
  });
}

export async function mockOnboardingStatus(page: Page, completed: boolean) {
  await page.route("**/api/v1/onboarding/status", (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(json({ completed }));
  });
}

export async function mockAuthMe(page: Page, user: MockUser | null) {
  await page.route("**/api/v1/auth/me", (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    if (!user) {
      return route.fulfill({ status: 401, ...json({ error: "Unauthorized" }) });
    }
    return route.fulfill(dataEnvelope(user));
  });
}

export async function mockOAuthDisabled(page: Page) {
  await page.route("**/api/v1/auth/google/enabled", (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(json({ enabled: false }));
  });
  await page.route("**/api/v1/auth/slack/enabled", (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(json({ enabled: false }));
  });
  await page.route("**/api/v1/auth/oidc/providers", (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope([]));
  });
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

export type MockAlert = {
  fingerprint: string;
  alert_number: number;
  status: string;
  acknowledged: boolean;
  silenced: boolean;
  labels: Record<string, string>;
  annotations: Record<string, string>;
  values: unknown;
  starts_at: string;
  ends_at: string | null;
  generator_url: string;
  events: unknown[];
  updated_at: string;
  created_at: string;
  deleted_at: string | null;
};

export function makeAlert(overrides: Partial<MockAlert> = {}): MockAlert {
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

export type MockIncident = {
  id: string;
  incident_number: number;
  title: string;
  description: string;
  status: string;
  severity: string;
  impact_level: string;
  priority: string;
  incident_type: string;
  slack_channel_archived: boolean;
  created_at: string;
  updated_at: string;
  deleted_at: string | null;
};

export function makeIncident(overrides: Partial<MockIncident> = {}): MockIncident {
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

export type MockInvestigation = {
  id: string;
  alert_investigation_id: string;
  alert_investigation_number: number;
  alerts: unknown[];
  correlation_key: string;
  status: string;
  agent_id: string;
  agent_name: string;
  agent_type: string;
  assignee_type: string;
  assignee_id: string;
  created_at: string;
  updated_at: string;
  investigating_duration_ms: number;
};

export function makeInvestigation(overrides: Partial<MockInvestigation> = {}): MockInvestigation {
  return {
    id: "inv-001",
    alert_investigation_id: "ainv-001",
    alert_investigation_number: 1,
    alerts: [],
    correlation_key: "corr-001",
    status: "investigating",
    agent_id: "agent-001",
    agent_name: "Hermes",
    agent_type: "hermes",
    assignee_type: "agent",
    assignee_id: "agent-001",
    created_at: "2025-06-01T10:00:00Z",
    updated_at: "2025-06-01T10:00:00Z",
    investigating_duration_ms: 0,
    ...overrides,
  };
}

export async function mockAlertDetailApis(
  page: Page,
  alertNumber: number,
  options: {
    alert?: MockAlert;
    investigation?: MockInvestigation | null;
    relatedAlerts?: unknown[];
    incident?: unknown;
    users?: unknown[];
  } = {},
) {
  const alert = options.alert ?? makeAlert({ alert_number: alertNumber });
  const investigation = options.investigation === undefined ? null : options.investigation;
  await page.route(`**/api/v1/alerts/${alertNumber}/related`, (route: Route) =>
    route.fulfill(
      dataEnvelope({
        related_alerts: options.relatedAlerts ?? [],
        incident: options.incident ?? null,
      }),
    ),
  );
  await page.route(`**/api/v1/alerts/${alertNumber}/thread**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope({ messages: [] }));
  });
  await page.route(`**/api/v1/alerts/${alertNumber}`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(
      dataEnvelope({
        alert,
        ...(investigation ? { alert_investigation: investigation } : {}),
      }),
    );
  });
  await page.route("**/api/v1/users**", (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope(options.users ?? [makeUser()]));
  });
  await page.route("**/api/v1/agent-tokens**", (route: Route) => route.fulfill(dataEnvelope([])));
  await page.route("**/api/v1/integrations**", (route: Route) => route.fulfill(dataEnvelope([])));
  await page.route("**/api/v1/notifications**", (route: Route) => route.fulfill(dataEnvelope([])));
  await page.route("**/api/v1/events**", (route: Route) => route.abort());
}

export async function mockIncidentDetailApis(
  page: Page,
  incidentNumber: number,
  options: {
    incident?: MockIncident;
    alerts?: unknown[];
    icsRoles?: unknown[];
    coordinationMessages?: unknown[];
    coordinationTasks?: unknown[];
    statusUpdates?: unknown[];
    thread?: unknown;
    document?: unknown[];
    users?: unknown[];
  } = {},
) {
  const incident = options.incident ?? makeIncident({ incident_number: incidentNumber });
  await page.route(`**/api/v1/incidents/${incidentNumber}/alerts**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope(options.alerts ?? []));
  });
  await page.route(`**/api/v1/incidents/${incidentNumber}/ics/roles**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope(options.icsRoles ?? []));
  });
  await page.route(`**/api/v1/incidents/${incidentNumber}/ics/document**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope(options.document ?? []));
  });
  await page.route(
    `**/api/v1/incidents/${incidentNumber}/coordination/messages**`,
    (route: Route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill(dataEnvelope(options.coordinationMessages ?? []));
    },
  );
  await page.route(`**/api/v1/incidents/${incidentNumber}/coordination/tasks**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope(options.coordinationTasks ?? []));
  });
  await page.route(`**/api/v1/incidents/${incidentNumber}/status-updates**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope(options.statusUpdates ?? []));
  });
  await page.route(`**/api/v1/incidents/${incidentNumber}/thread**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(
      dataEnvelope(options.thread ?? { messages: [], thread_id: "t-1", provider: "internal" }),
    );
  });
  await page.route(`**/api/v1/incidents/${incidentNumber}/timeline**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope([]));
  });
  await page.route(`**/api/v1/incidents/${incidentNumber}/post-mortem**`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill({ status: 404, ...json({ error: "Not found" }) });
  });
  await page.route(`**/api/v1/incidents/${incidentNumber}`, (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope({ incident }));
  });
  await page.route("**/api/v1/users**", (route: Route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return route.fulfill(dataEnvelope(options.users ?? [makeUser()]));
  });
  await page.route("**/api/v1/agent-tokens**", (route: Route) => route.fulfill(dataEnvelope([])));
  await page.route("**/api/v1/integrations**", (route: Route) => route.fulfill(dataEnvelope([])));
  await page.route("**/api/v1/notifications**", (route: Route) => route.fulfill(dataEnvelope([])));
  await page.route("**/api/v1/playbooks**", (route: Route) =>
    route.fulfill(dataEnvelope({ items: [], total: 0 })),
  );
  await page.route("**/api/v1/events**", (route: Route) => route.abort());
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
