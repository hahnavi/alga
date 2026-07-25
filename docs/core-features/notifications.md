---
title: Notifications
description: Alga's notification system — multi-channel dispatch, @mentions, delivery logs, a daily AI-generated summary, and global search across alerts, incidents, and investigations.
---

# In-App Notifications

Alga provides a built-in notification system with per-user preferences, multi-channel dispatch, and real-time delivery.

## How Notifications Work

1. A trigger event occurs (alert, investigation update, escalation, mention, on-call handoff)
2. The notification dispatcher resolves user preferences and channel settings
3. Notifications are dispatched to all enabled channels in parallel
4. Delivery status is logged per notification per channel

## Notification Triggers

| Trigger | Description |
|---------|-------------|
| Alert created | New firing alert received |
| Alert acknowledged | Alert marked as acknowledged |
| Alert resolved | Alert resolved |
| Investigation created | New investigation dispatched |
| Investigation updated | Agent sends update |
| Investigation complete | Investigation finished |
| Incident created | New incident detected |
| Incident acknowledged | Incident acknowledged |
| Incident mitigated | Incident mitigated |
| Incident resolved | Incident resolved |
| Escalation triggered | Escalation policy fires |
| On-call reminder | Upcoming shift reminder |
| On-call handoff | Shift change notification |
| `@mention` | User mentioned in a comment |

## Multi-Channel Dispatch

Each notification is dispatched to all channels enabled in the user's preferences:

| Channel | Description |
|---------|-------------|
| In-app (`in_app`) | Notification bell dropdown, pushed via SSE |
| Email (`email`) | SMTP-delivered email notification |
| Slack DM (`slack`) | Direct message via Slack bot |
| Voice (`voice`) | Phone call with IVR menu (requires a phone number; respects voice opt-out) |
| Mattermost (`mattermost`) | Placeholder — accepted but not yet delivered |

The dispatcher resolves each user's preferences and dispatches to all enabled channels. If a channel is unavailable (e.g., Slack not configured), it is skipped and logged.

## @Mentions

Type `@` in investigation comments to mention users:

- `@username` — Notifies the specific user

Mention handling resolves only users (and agents) — there is no concept of "groups" in Alga. To address multiple responders at once, use the on-call schedules and escalation policies configured for your teams. Mentions create in-app notifications and deliver to the user's preferred channels.

## Notification Bell

The notification bell in the top bar shows unread count and a dropdown with recent notifications. Click any notification to navigate to the related resource.

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/notifications` | List notifications (limit, skip) |
| `GET` | `/api/v1/notifications/unread-count` | Get unread count |
| `POST` | `/api/v1/notifications/read-all` | Mark all as read |
| `POST` | `/api/v1/notifications/{id}/read` | Mark one as read |

## Real-Time Delivery

Notifications are pushed to the frontend in real-time via SSE (`GET /api/v1/events`). No polling required — the unread count updates instantly.

## Notification Preferences

Each user configures their own preferences as a set of rules mapping a `notification_type` to one or more channels (`in_app`, `email`, `slack`, `voice`), with a `default_channel` fallback for types without an explicit rule. A separate per-user **voice opt-out** flag suppresses voice calls regardless of rules.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/users/me/notification-preferences` | Get preferences |
| `PUT` | `/api/v1/users/me/notification-preferences` | Update preferences |
| `POST` | `/api/v1/users/me/notification-preferences/test` | Send test notification |

See [Notification Preferences](/on-call/notification-preferences) for details.

## Delivery Log

Every notification attempt is logged with:
- Target user
- Channel used (in-app, email, mattermost, slack)
- Delivery status (sent, failed, skipped)
- Timestamp
- Error details (if failed)

The delivery log is stored in the `notification_delivery_logs` table and is queryable by user, incident, notification type, and channel.

## On-Call Handoff Notifications

When on-call shifts change, Alga automatically sends handoff notifications:
- **Incoming responder** — Notified that their shift has started with schedule name
- **Outgoing responder** — Notified that their shift has ended and prompted to write handoff notes

Handoff records track the outgoing and incoming users, handoff notes, and acknowledgement status. These are powered by the `HandoffDetector` which runs on the scheduler tick cycle.

## Daily AI Summary

Alga generates a daily operations digest using an LLM. The summary includes alert counts, active incidents, resolved issues, and trend analysis for the past 24 hours.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/dashboard/daily-summary` | Get the current daily summary |
| `POST` | `/api/v1/dashboard/daily-summary` | Force regeneration |

The `DailySummaryScheduler` automatically regenerates the summary when it becomes stale. The dashboard page displays the summary prominently. Generation requires `MEMORY_LLM_*` configuration for the LLM backend.

## Global Search

Press `Ctrl+K` (or `Cmd+K` on macOS) anywhere in the UI to open the global search. Search across:
- Alerts (by name, labels, summary)
- Incidents (by title, description)
- Investigations (by alert name, findings)

The search input is also available on the Dashboard page for quick access to any resource.
