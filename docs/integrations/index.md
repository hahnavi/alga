---
title: Integrations
description: Connect Alga to chat platforms (Slack, Mattermost), voice escalation (Twilio, Telnyx), email, and OIDC SSO. AI agents have their own dedicated section.
---

# Integrations

Alga integrates with your existing communication, monitoring, and identity tools. This section covers chat platforms, voice escalation, email, and single sign-on.

## AI Agents

AI agents are first-class in Alga and have a dedicated **[Agents section](/agents/)** covering the native [Alga Agent](/agents/alga-agent), the [Hermes](/agents/hermes) and [OpenClaw](/agents/openclaw) plugins, the [Agent SDKs](/agents/agent-sdks), plus [Agent Memory](/agents/memory), [Peer Ask](/agents/peer-ask), [Knowledge Base](/agents/knowledge-base), and [Credential Providers](/agents/credential-providers).

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
