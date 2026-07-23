import { computed, type Component } from "vue";
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Bell,
  BookOpen,
  Bot,
  Brain,
  Clock,
  FileText,
  HeartPulse,
  KeyRound,
  Layers,
  PlugZap,
  Route,
  Settings,
  Shield,
  ShieldCheck,
  Users,
} from "@lucide/vue";
import { useAuthStore } from "@/stores/auth";

/**
 * Single source of truth for the app's navigation structure.
 *
 * Three consumers read from here:
 *   - `Sidebar.vue` (full desktop nav) → `sidebarSections`
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
      readHeartbeats: auth.hasPermission("heartbeats:read"),
      readStatuspages: auth.hasPermission("statuspages:read"),
      manageOidc: auth.hasPermission("oidc:manage"),
      manageUsers: auth.hasPermission("users:manage"),
      readSystem: auth.hasPermission("system:read"),
    };

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
        label: "Automate",
        items: [buildAgentsGroup(p.manageAgents, p.readCreds)],
      },
      {
        label: "Operate",
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
      {
        label: "Configure",
        items: [
          { to: "/routes", icon: Route, label: "Routes" },
          { to: "/integrations", icon: PlugZap, label: "Integrations" },
          ...(p.readHeartbeats
            ? [{ to: "/heartbeats", icon: HeartPulse, label: "Heartbeats" }]
            : []),
          ...(p.readStatuspages
            ? [{ to: "/status-pages", icon: Activity, label: "Status Pages" }]
            : []),
          ...(p.manageOidc ? [{ to: "/sso", icon: KeyRound, label: "SSO" }] : []),
          ...(p.manageUsers ? [{ to: "/users", icon: Users, label: "Users" }] : []),
          ...(p.readSystem ? [{ to: "/system", icon: Settings, label: "System" }] : []),
        ],
      },
    ];
  });

  const mobileMoreSections = computed<NavSectionFlat[]>(() => {
    const p = {
      readPostmortem: auth.hasPermission("postmortems:read"),
      writeOncall: auth.hasPermission("oncall:write"),
      writeRoutes: auth.hasPermission("routes:write"),
      readHeartbeats: auth.hasPermission("heartbeats:read"),
      readStatuspages: auth.hasPermission("statuspages:read"),
      manageOidc: auth.hasPermission("oidc:manage"),
      manageUsers: auth.hasPermission("users:manage"),
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
    if (p.writeOncall) {
      operateItems.push({ to: "/escalation-policies", icon: Shield, label: "Escalation" });
    }
    if (p.writeRoutes) {
      operateItems.push({ to: "/maintenance", icon: ShieldCheck, label: "Maintenance" });
    }
    result.push({ label: "Operate", items: operateItems });

    const configureItems: NavFlat[] = [
      { to: "/routes", icon: Route, label: "Routes" },
      { to: "/integrations", icon: PlugZap, label: "Integrations" },
    ];
    if (p.readHeartbeats) {
      configureItems.push({ to: "/heartbeats", icon: HeartPulse, label: "Heartbeats" });
    }
    if (p.readStatuspages) {
      configureItems.push({ to: "/status-pages", icon: Activity, label: "Status Pages" });
    }
    if (p.manageOidc) {
      configureItems.push({ to: "/sso", icon: KeyRound, label: "SSO" });
    }
    if (p.manageUsers) {
      configureItems.push({ to: "/users", icon: Users, label: "Users" });
    }
    if (p.readSystem) {
      configureItems.push({ to: "/system", icon: Settings, label: "System" });
    }
    result.push({ label: "Configure", items: configureItems });

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
    mobileMoreSections,
    mobileAgentItems,
    mobileMorePaths,
    topNavItems: TOP_NAV_ITEMS,
  };
}
