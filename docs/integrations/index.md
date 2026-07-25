---
title: Integrations
description: Connect Alga to AI agents (Hermes, OpenClaw, custom SDKs), chat platforms (Slack, Mattermost), voice escalation (Twilio, Telnyx), email, and OIDC SSO.
---

# Integrations

Alga integrates with your existing communication, monitoring, and identity tools. This section covers everything from AI investigation agents to chat platforms, voice escalation, email, and single sign-on.

## AI Agents

Alga connects to autonomous AI SRE agents for automated alert investigation and root cause analysis. Agents connect via SSE, receive investigation dispatches, reason about the problem using built-in tools, and report findings back through the REST API. The scheduler is agent-type-agnostic — any online agent with the right capabilities and scope can win a dispatch.

| Integration | Language | Tools | Description |
|-------------|----------|-------|-------------|
| [Hermes Agent](/integrations/hermes) | Python | 31 | Nous Research's autonomous agent platform — the default agent type in Alga |
| [OpenClaw](/integrations/openclaw) | TypeScript | 32 | Alternative agent runtime with built-in memory and peer-ask tools |
| [Agent SDKs](/integrations/agent-sdks) | Go, JS, Python, Rust | — | Build a fully custom agent against the SSE + REST protocol |

→ See [AI Investigation](/core-features/investigation) for how the dispatch pipeline works end-to-end.

## Chat Platforms

### Slack

Full Slack workspace integration with bot-driven alert notifications, bidirectional thread sync, automated incident channels, and OAuth workspace installation.

- [Slack Integration](/integrations/slack) — Bot setup, Events API, incident channels, and user-level binding
- [Slack OAuth Setup](/integrations/slack-oauth) — Multi-workspace OAuth 2.0 installation

**User-level Slack binding** allows individual users to link their personal Slack account for DM notifications via the profile settings page.

### Mattermost

Plugin-based integration with bidirectional sync between Alga investigations and Mattermost threads.

- [Mattermost Integration](/integrations/mattermost) — Plugin installation, webhook configuration, and bidirectional sync

## Voice Escalation

Voice-call escalation pages the on-call user's phone with an IVR menu to acknowledge or silence the alert. Two providers are supported (mutually exclusive, selected by `VOICE_PROVIDER`):

- [Twilio Integration](/integrations/twilio) — Voice calls via Twilio (default)
- [Telnyx Integration](/integrations/telnyx) — Alternative voice provider via Telnyx Call Control API

## Email

SMTP-based email notifications for alert delivery, investigation updates, and password reset flows. Supports HTML templates with configurable from address.

- [Email Integration](/integrations/email) — SMTP configuration and notification templates

## Authentication

- [OIDC SSO](/integrations/oidc-sso) — Single sign-on via Okta, Keycloak, Google, Auth0, and other OIDC IdPs

## Notification Dispatcher

Alga uses a central notification dispatcher that resolves per-user preferences and fans out to multiple channels:

| Channel | Delivery | Notes |
|---------|----------|-------|
| `in_app` | SSE (real-time browser push) | Always available |
| `email` | SMTP via `EmailWorker` | Requires `SMTP_HOST` |
| `slack` | DM to user's linked Slack account | Requires user-level Slack binding |
| `voice` | Phone call via Twilio or Telnyx | Per-incident-user-level Valkey dedup; users can opt out |
| `mattermost` | Placeholder | Not yet fully implemented |

Users configure which channels receive which notification types via **Profile → Notification Preferences**.

## Alert Sources

Alga accepts alerts from any source that can send HTTP webhooks. Built-in compatibility with:

- Grafana Alerting
- Prometheus Alertmanager
- Any custom webhook source

See [Alerts](/core-features/alerts) for webhook setup instructions and payload format.
