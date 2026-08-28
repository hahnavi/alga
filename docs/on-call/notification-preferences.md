---
title: Notification Preferences
description: Per-user rule-based notification preferences — choose which channels receive which notification types, with a default channel fallback.
---

# Notification Preferences

Notification preferences let each user control **which channels** they receive alerts through and **for what types of events**. Preferences are rule-based — not simple toggles — so you can route incident escalations to Slack, acknowledgment receipts to email, and routine updates to in-app only.

## How Preferences Work

Each user has a set of rules that map notification types to channels, plus a default channel for anything not explicitly covered:

```
  Escalation triggered ────► Rule: escalation ──────────► [slack, email, voice]
                                        │
  Action item assigned ────► Rule: action_item_assigned ► [in_app, email]
                                        │
  Anything else ───────────► (no rule) ────────────────► Default: [in_app]
```

When an event triggers a notification, Alga checks your preference rules **in order** for one whose `notification_type` matches the event's type exactly or via the `*` wildcard — the first match wins. Rules whose `enabled` toggle is off are skipped entirely: a disabled rule behaves as if it did not exist. If no enabled rule matches, the notification goes to your `default_channel`.

## Available Channels

| Channel                       | Description                                            | Requires                                                                    |
| ----------------------------- | ------------------------------------------------------ | --------------------------------------------------------------------------- |
| **In-App** (`in_app`)         | Notifications appear in the notification bell dropdown | Nothing (always available)                                                  |
| **Email** (`email`)           | Sent via the configured SMTP server                    | [Email integration](/integrations/email) set up                             |
| **Slack** (`slack`)           | Sent to your linked Slack account via DM               | [Slack integration](/integrations/slack) + account linked                   |
| **Voice** (`voice`)           | Phone call with IVR menu to acknowledge                | [Twilio](/integrations/twilio) or [Telnyx](/integrations/telnyx) configured |
| **Mattermost** (`mattermost`) | Placeholder — accepted in rules but not yet delivered  | [Mattermost integration](/integrations/mattermost)                          |

::: tip Link your accounts first
The Slack channel only works if you've linked your personal Slack account. Go to **Profile → Connected Accounts** to link Slack before configuring that channel in your preferences.
:::

::: warning Voice opt-out
Each user has a separate **voice opt-out** flag (`voice_opt_out`). When enabled, voice calls are suppressed for that user even if a rule or escalation level includes the `voice` channel — useful for cost control or personal preference. Voice calls also require a phone number on file.
:::

## Preference Structure

```json
{
  "rules": [
    {
      "notification_type": "escalation",
      "channels": ["in_app", "email", "slack", "voice"],
      "enabled": true
    },
    {
      "notification_type": "mention",
      "channels": ["in_app", "slack"],
      "enabled": true
    },
    {
      "notification_type": "*",
      "channels": ["in_app"],
      "enabled": true
    }
  ],
  "default_channel": "in_app"
}
```

`default_channel` is optional; when unset (or set to `none` in the UI), unmatched notifications fall back to in-app delivery.

## Notification Types

Only these types are emitted today; they match the options in the UI. Configuring other type names is possible via the API but such rules stay inert until producers emit those events.

| Notification Type                                                  | When It Fires                                       | Recommended Channels           |
| ------------------------------------------------------------------ | --------------------------------------------------- | ------------------------------ |
| `escalation`                                                       | An escalation policy fires                          | In-app, Slack, Email, Voice    |
| `oncall_handoff`                                                   | Your on-call shift starts or ends                   | In-app, Email                  |
| `oncall_reminder`                                                  | Your shift starts soon (~15 min ahead of handover)  | In-app, Push-style in-app ping |
| `post_mortem_review_requested`                                     | A post-mortem is submitted for review               | In-app, Email                  |
| `action_item_assigned`                                             | An action item is assigned to you                   | In-app, Email                  |
| `mention`                                                          | Someone @mentions you in an investigation thread    | In-app, Slack                  |
| `info`                                                             | Action-item due-date reminder sweep                 | In-app                         |
| `incident_acknowledged` / `_mitigated` / `_resolved` / `_reopened` | An incident you command or respond on changes state | In-app, Slack, Email           |
| `*`                                                                | Wildcard — matches any type                         | In-app                         |

::: warning More triggers are on the roadmap
Alert lifecycle events (created / acknowledged / resolved) and investigation updates are **planned**, not shipped — they need digest/rate-limit design first. See [Notifications](/core-features/notifications#notification-triggers) for current status.
:::

## Managing Your Preferences

1. Click your **profile avatar** in the top-right corner
2. Select **Notification Preferences**
3. Set your **default channel** (used when no rule matches)
4. Add or edit rules — pick a notification type, toggle the channels you want, and switch each rule on or off
5. Click **Save**
6. Use **Test** to verify in-app delivery works

::: warning Test notifications are in-app only
The **Send Test** button (`POST /api/v1/users/me/notification-preferences/test`) is idempotent and creates an in-app test notification — it does not exercise the email, Slack, or voice pipelines. To verify those channels are working, trigger a real notification (e.g., create a test incident assigned to yourself).
:::

## Best Practices

- **Reserve voice for escalations** — phone calls are disruptive. Only enable voice on your `escalation` rule, not on `*`
- **Use Slack for real-time awareness** — keep mentions and escalations mirrored to Slack so you see them where you already work
- **Keep a default channel** — set `default_channel` so you never miss a type you forgot to configure
- **Disable instead of delete** — turning a rule's enabled toggle off silences it without losing the configuration; re-enable it later

::: tip Severity and time-window filters are not evaluated yet
Rule fields for severity filters and active time windows (`severity_filter`, `start_time`, `end_time`) exist in the payload but are ignored by the dispatcher today — planned for phase 2 alongside digest-style triggers.
:::

## API Endpoints

| Method | Path                                             | Auth    | Description                                      |
| ------ | ------------------------------------------------ | ------- | ------------------------------------------------ |
| `GET`  | `/api/v1/users/me/notification-preferences`      | Session | Get current user's preferences                   |
| `PUT`  | `/api/v1/users/me/notification-preferences`      | Session | Update preferences                               |
| `POST` | `/api/v1/users/me/notification-preferences/test` | Session | Send test notification (in-app only, idempotent) |

## See Also

- [Teams](/on-call/) — team structure and membership
- [Escalation Policies](/on-call/escalation-policies) — multi-tier escalation chains
- [On-Call Schedules](/on-call/schedules) — rotating coverage
- [Notifications](/core-features/notifications) — the notification system overview
