import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { pageTitleForPath } from "@/lib/pageTitles";

declare module "vue-router" {
  interface RouteMeta {
    public?: boolean;
    guestOnly?: boolean;
    requiredPermission?: string;
    area?: "settings";
  }
}

const DashboardPage = () => import("@/pages/DashboardPage.vue");
const AlertsPage = () => import("@/pages/AlertsPage.vue");
const RoutesPage = () => import("@/pages/RoutesPage.vue");
const CommunicationChannelsPage = () => import("@/pages/CommunicationChannelsPage.vue");
const IncomingWebhooksPage = () => import("@/pages/IncomingWebhooksPage.vue");
const LoginPage = () => import("@/pages/LoginPage.vue");
const UsersPage = () => import("@/pages/UsersPage.vue");
const AlertDetailPage = () => import("@/pages/AlertDetailPage.vue");
const AgentPrivateChatPage = () => import("@/pages/AgentPrivateChatPage.vue");
const AgentsPage = () => import("@/pages/AgentsPage.vue");
const NotificationsPage = () => import("@/pages/NotificationsPage.vue");
const KnowledgePage = () => import("@/pages/KnowledgePage.vue");
const MemoryPage = () => import("@/pages/MemoryPage.vue");
const SystemGeneralPage = () => import("@/pages/system/SystemGeneralPage.vue");
const SystemInvestigationsPage = () => import("@/pages/system/SystemInvestigationsPage.vue");
const SystemIncidentsPage = () => import("@/pages/system/SystemIncidentsPage.vue");
const MaintenanceWindowsPage = () => import("@/pages/MaintenanceWindowsPage.vue");
const HeartbeatsPage = () => import("@/pages/HeartbeatsPage.vue");
const StatusPagesPage = () => import("@/pages/StatusPagesPage.vue");
const StatusPageViewPage = () => import("@/pages/StatusPageViewPage.vue");
const OIDCProvidersPage = () => import("@/pages/OIDCProvidersPage.vue");
const IncidentsPage = () => import("@/pages/IncidentsPage.vue");
const SharedSecretsPage = () => import("@/pages/SharedSecretsPage.vue");
const CredentialProvidersPage = () => import("@/pages/CredentialProvidersPage.vue");
const IncidentDetailPage = () => import("@/pages/IncidentDetailPage.vue");
const ServicesPage = () => import("@/pages/ServicesPage.vue");
const ServiceDetailPage = () => import("@/pages/ServiceDetailPage.vue");
const TeamsPage = () => import("@/pages/TeamsPage.vue");
const TeamDetailPage = () => import("@/pages/TeamDetailPage.vue");
const OnCallPage = () => import("@/pages/OnCallPage.vue");
const ScheduleEditorPage = () => import("@/pages/ScheduleEditorPage.vue");
const EscalationPoliciesPage = () => import("@/pages/EscalationPoliciesPage.vue");
const PostMortemPage = () => import("@/pages/PostMortemPage.vue");
const PostMortemsPage = () => import("@/pages/PostMortemsPage.vue");
const NotificationPreferencesPage = () => import("@/pages/NotificationPreferencesPage.vue");
const PersonalAccessTokensPage = () => import("@/pages/PersonalAccessTokensPage.vue");
const PlaybookPage = () => import("@/pages/PlaybookPage.vue");
const PlaybookDetailPage = () => import("@/pages/PlaybookDetailPage.vue");
const SettingsGeneralPage = () => import("@/pages/settings/SettingsGeneralPage.vue");
const SettingsAppearancePage = () => import("@/pages/settings/SettingsAppearancePage.vue");
const SettingsSecurityPage = () => import("@/pages/settings/SettingsSecurityPage.vue");
const SettingsIntegrationsPage = () => import("@/pages/settings/SettingsIntegrationsPage.vue");
const SettingsAuthenticationPage = () => import("@/pages/settings/SettingsAuthenticationPage.vue");

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/setup",
      component: () => import("@/pages/SetupPage.vue"),
      meta: { guestOnly: true, public: true },
    },
    {
      path: "/onboarding",
      component: () => import("@/pages/OnboardingPage.vue"),
    },
    {
      path: "/login",
      component: () => import("@/pages/AuthShell.vue"),
      children: [
        { path: "", component: LoginPage, meta: { public: true, guestOnly: true } },
        {
          path: "/forgot-password",
          component: () => import("@/pages/ForgotPasswordPage.vue"),
          meta: { public: true },
        },
        {
          path: "/reset-password",
          component: () => import("@/pages/ResetPasswordPage.vue"),
          meta: { public: true },
        },
      ],
    },
    {
      path: "/settings",
      redirect: (to) => {
        if (to.query.slack_linked || to.query.google_linked) {
          return { path: "/settings/integrations", query: to.query };
        }
        const tab = to.query.settings;
        const query = { ...to.query };
        delete query.settings;
        if (tab === "general" || tab === "appearance" || tab === "security") {
          return { path: `/settings/${tab}`, query };
        }
        if (tab === "integrations") {
          return { path: "/settings/integrations", query };
        }
        return { path: "/settings/general", query };
      },
    },
    {
      path: "/settings/general",
      component: SettingsGeneralPage,
      meta: { area: "settings" },
    },
    {
      path: "/settings/appearance",
      component: SettingsAppearancePage,
      meta: { area: "settings" },
    },
    {
      path: "/settings/security",
      component: SettingsSecurityPage,
      meta: { area: "settings" },
    },
    {
      path: "/settings/integrations",
      component: SettingsIntegrationsPage,
      meta: { area: "settings" },
    },
    {
      path: "/settings/access-tokens",
      component: PersonalAccessTokensPage,
      meta: { requiredPermission: "tokens:manage", area: "settings" },
    },
    {
      path: "/settings/notifications",
      component: NotificationPreferencesPage,
      meta: { requiredPermission: "notifications:read", area: "settings" },
    },
    {
      path: "/settings/routes",
      component: RoutesPage,
      meta: { requiredPermission: "routes:read", area: "settings" },
    },
    {
      path: "/settings/heartbeats",
      component: HeartbeatsPage,
      meta: { requiredPermission: "heartbeats:read", area: "settings" },
    },
    {
      path: "/settings/status-pages",
      component: StatusPagesPage,
      meta: { requiredPermission: "statuspages:read", area: "settings" },
    },
    {
      path: "/settings/credential-providers",
      component: CredentialProvidersPage,
      meta: { requiredPermission: "credentials:read", area: "settings" },
    },
    {
      path: "/settings/sso",
      component: OIDCProvidersPage,
      meta: { requiredPermission: "oidc:manage", area: "settings" },
    },
    {
      path: "/settings/authentication",
      component: SettingsAuthenticationPage,
      meta: { requiredPermission: "system:read", area: "settings" },
    },
    {
      path: "/settings/users",
      component: UsersPage,
      meta: { requiredPermission: "users:manage", area: "settings" },
    },
    { path: "/personal-access-tokens", redirect: { path: "/settings/access-tokens" } },
    { path: "/notification-preferences", redirect: { path: "/settings/notifications" } },
    { path: "/routes", redirect: { path: "/settings/routes" } },
    { path: "/heartbeats", redirect: { path: "/settings/heartbeats" } },
    { path: "/status-pages", redirect: { path: "/settings/status-pages" } },
    { path: "/credential-providers", redirect: { path: "/settings/credential-providers" } },
    { path: "/sso", redirect: { path: "/settings/sso" } },
    { path: "/users", redirect: { path: "/settings/users" } },
    { path: "/", component: DashboardPage },
    { path: "/alerts", component: AlertsPage, meta: { requiredPermission: "alerts:read" } },
    {
      path: "/alerts/:alertNumber",
      component: AlertDetailPage,
      meta: { requiredPermission: "alerts:read" },
    },
    {
      path: "/communication-channels",
      component: CommunicationChannelsPage,
      meta: { requiredPermission: "integrations:read" },
    },
    {
      path: "/incoming-webhooks",
      component: IncomingWebhooksPage,
      meta: { requiredPermission: "tokens:manage" },
    },
    { path: "/integrations", redirect: { path: "/communication-channels" } },
    { path: "/agents", component: AgentsPage, meta: { requiredPermission: "tokens:manage" } },
    {
      path: "/agents/:agent_token_id/chat",
      component: AgentPrivateChatPage,
      meta: { requiredPermission: "tokens:manage" },
    },
    {
      path: "/incidents",
      component: IncidentsPage,
      meta: { requiredPermission: "incidents:read" },
    },
    {
      path: "/incidents/metrics",
      redirect: { path: "/", query: { tab: "metrics" } },
    },
    {
      path: "/incidents/:incident_number",
      component: IncidentDetailPage,
      meta: { requiredPermission: "incidents:read" },
    },
    { path: "/services", component: ServicesPage, meta: { requiredPermission: "services:read" } },
    {
      path: "/services/:service_id",
      component: ServiceDetailPage,
      meta: { requiredPermission: "services:read" },
    },
    { path: "/teams", component: TeamsPage, meta: { requiredPermission: "oncall:read" } },
    { path: "/teams/:id", component: TeamDetailPage, meta: { requiredPermission: "oncall:read" } },
    { path: "/on-call", component: OnCallPage, meta: { requiredPermission: "oncall:read" } },
    {
      path: "/on-call/schedules/:id",
      component: ScheduleEditorPage,
      meta: { requiredPermission: "oncall:read" },
    },
    {
      path: "/escalation-policies",
      component: EscalationPoliciesPage,
      meta: { requiredPermission: "escalation:read" },
    },
    {
      path: "/incidents/:incident_number/post-mortem",
      component: PostMortemPage,
      meta: { requiredPermission: "postmortems:read" },
    },
    {
      path: "/post-mortems",
      component: PostMortemsPage,
      meta: { requiredPermission: "postmortems:read" },
    },
    {
      path: "/notifications",
      component: NotificationsPage,
      meta: { requiredPermission: "notifications:read" },
    },
    {
      path: "/knowledge",
      component: KnowledgePage,
      meta: { requiredPermission: "knowledge:read" },
    },
    {
      path: "/maintenance",
      component: MaintenanceWindowsPage,
      meta: { requiredPermission: "routes:write" },
    },
    {
      path: "/status/:slug",
      component: StatusPageViewPage,
      meta: { requiredPermission: "statuspages:read" },
    },
    {
      path: "/credentials",
      component: SharedSecretsPage,
      meta: { requiredPermission: "credentials:read" },
    },
    { path: "/memories", component: MemoryPage, meta: { requiredPermission: "memories:read" } },
    {
      path: "/system/general",
      component: SystemGeneralPage,
      meta: { requiredPermission: "system:read" },
    },
    {
      path: "/system/investigations",
      component: SystemInvestigationsPage,
      meta: { requiredPermission: "system:read" },
    },
    {
      path: "/system/incidents",
      component: SystemIncidentsPage,
      meta: { requiredPermission: "system:read" },
    },
    { path: "/system", redirect: { path: "/system/general" } },
    { path: "/playbooks", component: PlaybookPage, meta: { requiredPermission: "playbooks:read" } },
    {
      path: "/playbooks/:id",
      component: PlaybookDetailPage,
      meta: { requiredPermission: "playbooks:read" },
    },
    { path: "/:pathMatch(.*)*", component: () => import("@/pages/NotFoundPage.vue") },
  ],
});

router.beforeEach(async (to) => {
  const auth = useAuthStore();

  await auth.checkSetupStatus();

  // If setup is required, force all traffic to the setup wizard.
  if (auth.needsSetup === true && to.path !== "/setup") {
    return { path: "/setup" };
  }

  // Setup is done; don't let anyone (re)visit the setup page.
  if (auth.needsSetup === false && to.path === "/setup") {
    return { path: "/login" };
  }

  if (to.meta.guestOnly) {
    if (!auth.user && !auth.sessionChecked) {
      await auth.fetchCurrentUser();
    }
    if (auth.user) {
      return { path: "/" };
    }
    return true;
  }

  if (to.meta.public) return true;

  if (!auth.user) {
    await auth.fetchCurrentUser();
  }

  if (!auth.user) {
    return { path: "/login" };
  }

  if (auth.onboardingCompleted === false && to.path !== "/onboarding") {
    return { path: "/onboarding" };
  }

  if (to.meta.requiredPermission && !auth.hasPermission(to.meta.requiredPermission)) {
    return { path: "/" };
  }

  return true;
});

const DOCUMENT_TITLE_SUFFIX = "Alga";

function setDocumentTitle(label: string): void {
  document.title = label ? `${label} · ${DOCUMENT_TITLE_SUFFIX}` : DOCUMENT_TITLE_SUFFIX;
}

router.afterEach((to) => {
  setDocumentTitle(pageTitleForPath(to.path));
});

router.onError((error, to) => {
  const msg = error.message;
  const isStaleChunk =
    msg.includes("error loading dynamically imported module") ||
    msg.includes("Failed to fetch dynamically imported module") ||
    msg.includes("Unable to preload CSS");
  if (!isStaleChunk) return;
  const key = "alga_chunk_reload";
  if (sessionStorage.getItem(key) === to.fullPath) return;
  sessionStorage.setItem(key, to.fullPath);
  window.location.assign(to.fullPath);
});

export default router;
