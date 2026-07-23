---
title: Notification Preferences
description: Per-user rule-based notification preferences — choose which channels receive which notification types, with a default channel fallback.
---

# Notification Preferences

Notification preferences let each user control **which channels** they receive alerts through and **for what types of events**. Preferences are rule-based — not simple toggles — so you can route incident escalations to Slack, acknowledgment receipts to email, and routine updates to in-app only.

## How Preferences Work

Each user has a set of rules that map notification types to channels, plus a default channel for anything not explicitly covered:

```
  Incident escalated ──► Rule: [slack, email, voice]
                                │
  Incident acknowledged ──► Rule: [in_app, email]
                                │
  Alert resolved ──► (no rule) ──► Default: [in_app]
```

When an event triggers a notification, Alga checks the user's preference rules for a matching `notification_type`. If a rule matches, the notification goes to that rule's channels. If no rule matches, it goes to the `default_channel`.

## Available Channels

| Channel | Description | Requires |
|----------|-------------|----------|
| **In-App** | Notifications appear in the notification bell dropdown | Nothing (always available) |
| **Email** | Sent via the configured SMTP server | [Email integration](/integrations/email) set up |
| **Mattermost** | Sent to your linked Mattermost account | [Mattermost integration](/integrations/mattermost) + account linked |
| **Slack** | Sent to your linked Slack account via DM | [Slack integration](/integrations/slack) + account linked |
| **Voice** | Phone call with IVR menu to acknowledge | [Twilio](/integrations/twilio) or [Telnyx](/integrations/telnyx) configured |

::: tip Link your accounts first
Mattermost and Slack channels only work if you've linked your personal chat account. Go to **Profile → Connected Accounts** to link Slack or Mattermost before configuring those channels in your preferences.
:::

## Preference Structure

```json
{
  "rules": [
    {
      "notification_type": "incident_acknowledged",
      "channels": ["in_app", "email"]
    },
    {
      "notification_type": "incident_escalated",
      "channels": ["in_app", "email", "slack", "voice"]
    },
    {
      "notification_type": "alert_triggered",
      "channels": ["in_app"]
    }
  ],
  "default_channel": "in_app"
}
```

## Common Notification Types

These are the most commonly configured notification types. The full list is available in the UI.

| Notification Type | When It Fires | Recommended Channels |
|---|---|---|
| `incident_created` | A new incident is opened | In-app, Slack |
| `incident_escalated` | Escalation moved to the next level | In-app, Slack, Email, Voice |
| `incident_acknowledged` | Someone acknowledged the incident | In-app, Email |
| `incident_resolved` | The incident is marked resolved | In-app |
| `alert_triggered` | A new alert fired | In-app |
| `investigation_completed` | An AI investigation finished | In-app |

## Managing Your Preferences

1. Click your **profile avatar** in the top-right corner
2. Select **Notification Preferences**
3. Add or edit rules — pick a notification type and the channels you want
4. Set your **default channel** (used when no rule matches)
5. Click **Save Preferences**
6. Use **Send Test** to verify in-app delivery works

::: warning Test notifications are limited
The **Send Test** button only verifies in-app delivery — it does not test the full email, Slack, or Mattermost pipeline. To verify those channels are working, trigger a real notification (e.g., create a test incident assigned to yourself).
:::

## Best Practices

- **Reserve voice for escalations** — phone calls are disruptive. Only enable voice for `incident_escalated` at higher levels, not for every alert
- **Use Slack for real-time awareness** — keep your primary incident notifications in Slack so your team has shared visibility
- **Keep a default channel** — always set `default_channel` to `in_app` so you never miss a notification type you forgot to configure
- **Different rules for different severities** — route critical incidents (P1) to more channels than routine updates

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/users/me/notification-preferences` | Session | Get current user's preferences |
| `PUT` | `/api/v1/users/me/notification-preferences` | Session | Update preferences |
| `POST` | `/api/v1/users/me/notification-preferences/test` | Session | Send test notification (in-app only) |

## See Also

- [Teams](/on-call/) — team structure and membership
- [Escalation Policies](/on-call/escalation-policies) — multi-tier escalation chains
- [On-Call Schedules](/on-call/schedules) — rotating coverage
- [Notifications](/core-features/notifications) — the notification system overview
