import { defineConfig } from "vitepress"

export default defineConfig({
  title: "Alga",
  description: "The open-source, AI-powered incident management platform",
  cleanUrls: true,
  ignoreDeadLinks: ["localhostLinks"],
  head: [
    ["link", { rel: "icon", type: "image/svg+xml", href: "/logo.svg" }],
  ],
  lastUpdated: true,
  themeConfig: {
    siteTitle: "Alga",
    logo: "/logo.svg",
    outline: {
      level: [2, 3],
      label: "On this page",
    },
    editLink: {
      pattern: "https://github.com/hahnavi/alga/edit/main/docs/:path",
      text: "Edit this page on GitHub",
    },
    nav: [
      { text: "Docs", link: "/getting-started" },
      { text: "API", link: "/api-reference" },
      { text: "GitHub", link: "https://github.com/hahnavi/alga" },
    ],
    sidebar: [
      {
        text: "Getting Started",
        collapsed: false,
        items: [
          { text: "Quick Start", link: "/getting-started/" },
          { text: "Core Concepts", link: "/getting-started/concepts" },
          { text: "Installation & Setup", link: "/getting-started/installation" },
          { text: "First Steps Guide", link: "/getting-started/first-steps" },
          { text: "Onboarding Wizard", link: "/getting-started/onboarding" },
        ],
      },
      {
        text: "Configuration",
        collapsed: true,
        items: [
          { text: "Environment Variables", link: "/configuration/environment-variables" },
          { text: "System Configuration API", link: "/configuration/system-config" },
          { text: "Security & Authentication", link: "/configuration/security" },
        ],
      },
      {
        text: "Integrations",
        collapsed: true,
        items: [
          { text: "Overview", link: "/integrations/" },
          { text: "Slack", link: "/integrations/slack" },
          { text: "Slack OAuth", link: "/integrations/slack-oauth" },
          { text: "Mattermost", link: "/integrations/mattermost" },
          { text: "Twilio", link: "/integrations/twilio" },
          { text: "Telnyx", link: "/integrations/telnyx" },
          { text: "Email", link: "/integrations/email" },
          { text: "OIDC SSO", link: "/integrations/oidc-sso" },
        ],
      },
      {
        text: "Core Features",
        collapsed: true,
        items: [
          { text: "Alerts", link: "/core-features/alerts" },
          { text: "Routing", link: "/core-features/routing" },
          { text: "AI Investigation", link: "/core-features/investigation" },
          { text: "Triage", link: "/core-features/triage" },
          { text: "Playbooks", link: "/core-features/playbooks" },
          { text: "Notifications", link: "/core-features/notifications" },
          { text: "Heartbeats", link: "/core-features/heartbeats" },
          { text: "Maintenance Windows", link: "/core-features/maintenance-windows" },
          { text: "Status Pages", link: "/core-features/status-pages" },
          { text: "Dashboard", link: "/core-features/dashboard" },
        ],
      },
      {
        text: "Agents",
        collapsed: false,
        items: [
          { text: "Overview", link: "/agents/" },
          { text: "Alga Agent", link: "/agents/alga-agent" },
          { text: "Hermes Agent", link: "/agents/hermes" },
          { text: "OpenClaw", link: "/agents/openclaw" },
          { text: "Agent SDKs", link: "/agents/agent-sdks" },
          { text: "Agent Memory", link: "/agents/memory" },
          { text: "Peer Ask", link: "/agents/peer-ask" },
          { text: "Knowledge Base", link: "/agents/knowledge-base" },
          { text: "Credential Providers", link: "/agents/credential-providers" },
        ],
      },
      {
        text: "Incident Management",
        collapsed: true,
        items: [
          { text: "Overview", link: "/incident-management/" },
          { text: "Lifecycle & States", link: "/incident-management/lifecycle" },
          { text: "ICS Roles", link: "/incident-management/ics-roles" },
          { text: "Coordination", link: "/incident-management/coordination" },
          { text: "On-Call Handoffs", link: "/incident-management/handoffs" },
          { text: "SLA Tracking", link: "/incident-management/sla" },
          { text: "Post-Mortems", link: "/incident-management/post-mortems" },
        ],
      },
      {
        text: "Service Management",
        collapsed: true,
        items: [
          { text: "Service Catalog", link: "/service-management/" },
        ],
      },
      {
        text: "On-Call & Escalation",
        collapsed: true,
        items: [
          { text: "Teams", link: "/on-call/" },
          { text: "On-Call Schedules", link: "/on-call/schedules" },
          { text: "Escalation Policies", link: "/on-call/escalation-policies" },
          { text: "Notification Preferences", link: "/on-call/notification-preferences" },
        ],
      },
      {
        text: "Operations",
        collapsed: true,
        items: [
          { text: "Architecture", link: "/operations/architecture" },
          { text: "Deployment", link: "/operations/deployment" },
          { text: "Performance & Scaling", link: "/operations/performance" },
          { text: "Monitoring & Observability", link: "/operations/monitoring" },
          { text: "Backup & Restore", link: "/operations/backup" },
          { text: "Migration Guide", link: "/operations/migration" },
          { text: "CLI Reference", link: "/operations/cli" },
          { text: "Personal Access Tokens", link: "/operations/personal-access-tokens" },
        ],
      },
      {
        text: "API Reference",
        collapsed: true,
        items: [
          { text: "Overview", link: "/api-reference/" },
        ],
      },
      {
        text: "Resources",
        collapsed: true,
        items: [
          { text: "Use Cases", link: "/resources/use-cases" },
          { text: "Contributing", link: "/resources/contributing" },
          { text: "FAQ", link: "/resources/faq" },
          { text: "Troubleshooting", link: "/resources/troubleshooting" },
        ],
      },
    ],
    search: {
      provider: "local",
    },
    socialLinks: [
      { icon: "github", link: "https://github.com/hahnavi/alga" },
    ],
    footer: {
      message: "Released under the MIT License.",
    },
  },
  vite: {
    server: {
      allowedHosts: true,
      port: 5174,
    }
  },
})
