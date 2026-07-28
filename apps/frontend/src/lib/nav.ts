import { computed, type Component } from "vue";
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Bell,
  BellRing,
  BookOpen,
  Bot,
  Brain,
  Clock,
  FileText,
  HeartPulse,
  Key,
  KeyRound,
  Layers,
  Link2,
  Lock,
  MessageSquare,
  Palette,
  Route,
  Shield,
  ShieldCheck,
  SlidersHorizontal,
  User,
  Users,
  Webhook,
  Workflow,
} from "@lucide/vue";
import { useAuthStore } from "@/stores/auth";

/**
 * Single source of truth for the app's navigation structure.
 *
 * Consumers:
 *   - `Sidebar.vue` (full desktop nav) → `sidebarSections`
 *   - `SettingsSidebar.vue` (settings-area desktop nav) → `settingsSections`
 *   - `MobileMoreMenu.vue` (the "More" bottom-sheet on mobile)
 *   - `MobileAgentMenu.vue` (the "Agents" bottom-sheet on mobile)
 *
 * The mobile bottom bar (Home / Alerts / Incidents) is the static
 * `TOP_NAV_ITEMS` list; the App.vue shell reads it directly because
 * the icons never change.
 *
 * Permission gating follows the same conventions the rest of the app
 * uses: only the explicit nav surfaces (`Sidebar`, `MobileAgentMenu`,
 * `MobileMoreMenu`, `App.vue`) are allowed to call `auth.hasPermission`
 * directly, and they all reach it through this module.
 */

export type NavChild = { to: string; icon: Component; label: string };
export type NavGroup = {
  icon: Component;
  label: string;
  defaultTo: string;
  children: NavChild[];
};
export type NavFlat = { to: string; icon: Component; label: string };
export type NavEntry = NavFlat | NavGroup;
export type NavSection = { label: string; items: NavEntry[] };
export type NavSectionFlat = { label: string; items: NavFlat[] };

export function isNavGroup(entry: NavEntry): entry is NavGroup {
  return "children" in entry;
}

export const TOP_NAV_ITEMS: ReadonlyArray<NavFlat> = [
  { to: "/", icon: BarChart3, label: "Home" },
  { to: "/alerts", icon: Bell, label: "Alerts" },
  { to: "/incidents", icon: AlertTriangle, label: "Incidents" },
];

const INCIDENTS_FLAT: NavFlat = {
  to: "/incidents",
  icon: AlertTriangle,
  label: "Incidents",
};

function buildAgentsGroup(canManageAgents: boolean, canReadCreds: boolean): NavGroup {
  const children: NavChild[] = [];
  if (canManageAgents) children.push({ to: "/agents", icon: Bot, label: "Agents" });
  children.push(
    { to: "/knowledge", icon: BookOpen, label: "Knowledge" },
    { to: "/memories", icon: Brain, label: "Memory" },
  );
  if (canReadCreds) children.push({ to: "/credentials", icon: KeyRound, label: "Secrets" });
  return {
    icon: Bot,
    label: "Agents",
    defaultTo: children[0].to,
    children,
  };
}

function buildSystemItems(): NavFlat[] {
  return [
    { to: "/system/general", icon: SlidersHorizontal, label: "General" },
    { to: "/system/investigations", icon: Workflow, label: "Investigations" },
    { to: "/system/incidents", icon: FileText, label: "Incidents" },
  ];
}

/**
 * Returns the four nav surfaces that depend on the auth store. The
 * `mobileMorePaths` set is exposed so the mobile bottom-bar can
 * mark the "More" tab active when the current route lives in the
 * More sheet.
 */
export function useNavSections() {
  const auth = useAuthStore();

  const sidebarSections = computed<NavSection[]>(() => {
    const p = {
      manageAgents: auth.hasPermission("tokens:manage"),
      readCreds: auth.hasPermission("credentials:read"),
      readPostmortem: auth.hasPermission("postmortems:read"),
      readPlaybooks: auth.hasPermission("playbooks:read"),
      readEscalation: auth.hasPermission("escalation:read"),
      writeRoutes: auth.hasPermission("routes:write"),
      readIntegrations: auth.hasPermission("integrations:read"),
      manageTokens: auth.hasPermission("tokens:manage"),
      readSystem: auth.hasPermission("system:read"),
    };

    const integrationsItems: NavFlat[] = [
      ...(p.readIntegrations
        ? [{ to: "/communication-channels", icon: MessageSquare, label: "Channels" }]
        : []),
      ...(p.manageTokens ? [{ to: "/incoming-webhooks", icon: Webhook, label: "Webhooks" }] : []),
    ];

    const configureItems: NavFlat[] = [...(p.readSystem ? buildSystemItems() : [])];

    return [
      {
        label: "Monitor",
        items: [
          { to: "/", icon: BarChart3, label: "Dashboard" },
          { to: "/alerts", icon: Bell, label: "Alerts" },
        ],
      },
      {
        label: "Respond",
        items: [
          INCIDENTS_FLAT,
          ...(p.readPostmortem
            ? [{ to: "/post-mortems", icon: FileText, label: "Post-Mortems" }]
            : []),
        ],
      },
      {
        label: "AI",
        items: [
          ...(p.manageAgents ? [{ to: "/agents", icon: Bot, label: "Agents" }] : []),
          { to: "/knowledge", icon: BookOpen, label: "Knowledge" },
          { to: "/memories", icon: Brain, label: "Memory" },
          ...(p.readCreds ? [{ to: "/credentials", icon: KeyRound, label: "Secrets" }] : []),
        ],
      },
      {
        label: "Operations",
        items: [
          { to: "/services", icon: Layers, label: "Services" },
          { to: "/on-call", icon: Clock, label: "On-Call" },
          { to: "/teams", icon: Users, label: "Teams" },
          ...(p.readPlaybooks ? [{ to: "/playbooks", icon: FileText, label: "Playbooks" }] : []),
          ...(p.readEscalation
            ? [{ to: "/escalation-policies", icon: Shield, label: "Escalation" }]
            : []),
          ...(p.writeRoutes
            ? [{ to: "/maintenance", icon: ShieldCheck, label: "Maintenance" }]
            : []),
        ],
      },
      ...(integrationsItems.length > 0
        ? [{ label: "Integrations", items: integrationsItems }]
        : []),
      ...(configureItems.length > 0 ? [{ label: "Configure", items: configureItems }] : []),
    ];
  });

  const settingsSections = computed<NavSectionFlat[]>(() => {
    const p = {
      manageTokens: auth.hasPermission("tokens:manage"),
      readNotifications: auth.hasPermission("notifications:read"),
      readCreds: auth.hasPermission("credentials:read"),
      readHeartbeats: auth.hasPermission("heartbeats:read"),
      readStatuspages: auth.hasPermission("statuspages:read"),
      manageOidc: auth.hasPermission("oidc:manage"),
      manageUsers: auth.hasPermission("users:manage"),
      readSystem: auth.hasPermission("system:read"),
    };

    const account: NavFlat[] = [
      { to: "/settings/general", icon: User, label: "General" },
      { to: "/settings/appearance", icon: Palette, label: "Appearance" },
      { to: "/settings/security", icon: Lock, label: "Security" },
      { to: "/settings/integrations", icon: Link2, label: "Connected Apps" },
      ...(p.manageTokens
        ? [{ to: "/personal-access-tokens", icon: Key, label: "Access Tokens" }]
        : []),
      ...(p.readNotifications
        ? [{ to: "/notification-preferences", icon: BellRing, label: "Notifications" }]
        : []),
    ];

    const workspace: NavFlat[] = [
      { to: "/routes", icon: Route, label: "Routes" },
      ...(p.readHeartbeats ? [{ to: "/heartbeats", icon: HeartPulse, label: "Heartbeats" }] : []),
      ...(p.readStatuspages
        ? [{ to: "/status-pages", icon: Activity, label: "Status Pages" }]
        : []),
      ...(p.readCreds
        ? [{ to: "/credential-providers", icon: KeyRound, label: "Credential Providers" }]
        : []),
      ...(p.manageOidc ? [{ to: "/sso", icon: KeyRound, label: "SSO" }] : []),
      ...(p.readSystem
        ? [{ to: "/settings/authentication", icon: ShieldCheck, label: "Authentication" }]
        : []),
      ...(p.manageUsers ? [{ to: "/users", icon: Users, label: "Users" }] : []),
    ];

    return [
      { label: "Account", items: account },
      { label: "Workspace", items: workspace },
    ];
  });

  const mobileMoreSections = computed<NavSectionFlat[]>(() => {
    const p = {
      readPostmortem: auth.hasPermission("postmortems:read"),
      readEscalation: auth.hasPermission("escalation:read"),
      writeRoutes: auth.hasPermission("routes:write"),
      readIntegrations: auth.hasPermission("integrations:read"),
      manageTokens: auth.hasPermission("tokens:manage"),
      readSystem: auth.hasPermission("system:read"),
    };

    const result: NavSectionFlat[] = [];

    const respondItems: NavFlat[] = [];
    if (p.readPostmortem) {
      respondItems.push({ to: "/post-mortems", icon: FileText, label: "Post-Mortems" });
    }
    if (respondItems.length > 0) {
      result.push({ label: "Respond", items: respondItems });
    }

    const operateItems: NavFlat[] = [
      { to: "/services", icon: Layers, label: "Services" },
      { to: "/on-call", icon: Clock, label: "On-Call" },
      { to: "/teams", icon: Users, label: "Teams" },
    ];
    if (p.readEscalation) {
      operateItems.push({ to: "/escalation-policies", icon: Shield, label: "Escalation" });
    }
    if (p.writeRoutes) {
      operateItems.push({ to: "/maintenance", icon: ShieldCheck, label: "Maintenance" });
    }
    result.push({ label: "Operations", items: operateItems });

    const integrationsItems: NavFlat[] = [];
    if (p.readIntegrations) {
      integrationsItems.push({
        to: "/communication-channels",
        icon: MessageSquare,
        label: "Channels",
      });
    }
    if (p.manageTokens) {
      integrationsItems.push({ to: "/incoming-webhooks", icon: Webhook, label: "Webhooks" });
    }
    if (integrationsItems.length > 0) {
      result.push({ label: "Integrations", items: integrationsItems });
    }

    const configureItems: NavFlat[] = [];
    if (p.readSystem) {
      configureItems.push(...buildSystemItems());
    }
    if (configureItems.length > 0) {
      result.push({ label: "Configure", items: configureItems });
    }

    result.push(...settingsSections.value);

    return result;
  });

  const mobileAgentItems = computed<NavChild[]>(() => {
    const canManageAgents = auth.hasPermission("tokens:manage");
    const canReadCreds = auth.hasPermission("credentials:read");
    return buildAgentsGroup(canManageAgents, canReadCreds).children;
  });

  const mobileMorePaths = computed<Set<string>>(() => {
    const paths = new Set<string>();
    for (const section of mobileMoreSections.value) {
      for (const item of section.items) {
        paths.add(item.to);
      }
    }
    return paths;
  });

  return {
    sidebarSections,
    settingsSections,
    mobileMoreSections,
    mobileAgentItems,
    mobileMorePaths,
    topNavItems: TOP_NAV_ITEMS,
  };
}
