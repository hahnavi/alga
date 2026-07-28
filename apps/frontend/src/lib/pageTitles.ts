const EXACT_TITLES: Record<string, string> = {
  "/": "Dashboard",
  "/alerts": "Alerts",
  "/routes": "Routes",
  "/communication-channels": "Communication Channels",
  "/incoming-webhooks": "Incoming Webhooks",
  "/agents": "Agents",
  "/users": "Users",
  "/incidents": "Incidents",
  "/incidents/metrics": "Incident Metrics",
  "/services": "Services",
  "/teams": "Teams",
  "/on-call": "On-Call",
  "/escalation-policies": "Escalation Policies",
  "/post-mortems": "Post-Mortems",
  "/notifications": "Notifications",
  "/notification-preferences": "Notification Preferences",
  "/knowledge": "Knowledge",
  "/memories": "Memories",
  "/maintenance": "Maintenance Windows",
  "/heartbeats": "Heartbeats",
  "/status-pages": "Status Pages",
  "/sso": "SSO Providers",
  "/credentials": "Shared Secrets",
  "/credential-providers": "Credential Providers",
  "/personal-access-tokens": "Personal Access Tokens",
  "/system/general": "System General",
  "/system/investigations": "System Investigations",
  "/system/incidents": "System Incidents",
  "/settings/general": "General Settings",
  "/settings/appearance": "Appearance",
  "/settings/security": "Security",
  "/settings/integrations": "Connected Apps",
  "/settings/authentication": "Authentication",
  "/playbooks": "Playbooks",
  "/login": "Sign in",
  "/setup": "Setup",
  "/forgot-password": "Forgot Password",
  "/reset-password": "Reset Password",
  "/onboarding": "Setup",
};

const PREFIX_TITLES: ReadonlyArray<readonly [prefix: string, title: string]> = [
  ["/alerts/", "Alert"],
  ["/incidents/", "Incident"],
  ["/services/", "Service"],
  ["/teams/", "Team"],
  ["/playbooks/", "Playbook"],
  ["/status/", "Status"],
];

export function pageTitleForPath(path: string): string {
  if (path.startsWith("/agents/") && path.endsWith("/chat")) return "Agent chat";
  if (path.endsWith("/post-mortem")) return "Post-Mortem";

  const exact = EXACT_TITLES[path];
  if (exact) return exact;

  for (const [prefix, title] of PREFIX_TITLES) {
    if (path.startsWith(prefix)) return title;
  }

  return "";
}
