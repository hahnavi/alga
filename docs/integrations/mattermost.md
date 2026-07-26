---
title: Mattermost Integration
description: Mattermost integration with plugin installation, webhook configuration, bidirectional sync between Alga investigations and Mattermost threads, channel mapping, and incident channels.
---

# Mattermost Integration

Alga integrates with [Mattermost](https://mattermost.com) for alert notifications, bidirectional thread sync, and team collaboration. When an alert fires, Alga posts a threaded message to the configured Mattermost channel. Operators can discuss and acknowledge directly from Mattermost, and the conversation syncs back to the Alga investigation thread in real time.

## How It Works

```
┌──────────┐   Alert notification (threaded message)   ┌────────────┐
│   Alga   │ ──────────────────────────────────────────► │  Mattermost │
│ Backend  │ ◄────────────────────────────────────────── │  Plugin     │
└──────────┘   Thread replies (webhook callback)        └────────────┘
```

1. An alert triggers a [routing rule](/core-features/routing) with a Mattermost destination
2. Alga posts a threaded message to the matching channel (or `MATTERMOST_DEFAULT_CHANNEL`)
3. The Mattermost plugin forwards thread replies back to Alga via webhook
4. Replies appear in the investigation thread — operators can discuss without leaving Mattermost
5. Agent findings and status updates are posted back to the Mattermost thread

## Prerequisites

- Mattermost Server v7+
- Admin access to install plugins
- A reachable Alga backend (the plugin needs to call back to it)

## Configuration

Set these environment variables in `apps/backend/.env`:

```sh
MATTERMOST_SERVER_URL=https://mattermost.example.com
MATTERMOST_WEBHOOK_SECRET=your-shared-secret
MATTERMOST_TEAM=engineering
MATTERMOST_DEFAULT_CHANNEL=alerts
```

| Variable                     | Required | Description                                                                                                                                                |
| ---------------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MATTERMOST_SERVER_URL`      | Yes      | Base URL of your Mattermost server. Alga appends `/plugins/com.alga.mattermost-plugin` for plugin API calls.                                               |
| `MATTERMOST_WEBHOOK_SECRET`  | Yes      | Shared secret used for both inbound webhook authentication and outbound plugin API authentication. Must match the value configured in the plugin settings. |
| `MATTERMOST_TEAM`            | Yes      | Mattermost team slug for channel resolution (e.g., `engineering`).                                                                                         |
| `MATTERMOST_DEFAULT_CHANNEL` | Yes      | Channel for alerts that don't match any routing rule (e.g., `alerts`).                                                                                     |
| `MATTERMOST_DISABLED`        | No       | Disable Mattermost delivery. Only settable via YAML config or the Integrations API — **not** as an env var. Defaults to `false`.                           |

::: tip Generate a strong webhook secret
Use `openssl rand -base64 32` to generate a secure `MATTERMOST_WEBHOOK_SECRET`. The same value must be entered in both the Alga env config and the Mattermost plugin settings.
:::

## Plugin Installation

The Alga Mattermost plugin enables bidirectional sync — without it, Alga can only post one-way notifications.

1. **Download the plugin** from your Alga instance: `http://your-alga-host:8080/internal/mm-plugin`
2. In Mattermost, go to **System Console → Plugins → Management**
3. **Upload** the plugin tarball
4. **Enable** the plugin
5. **Configure the plugin settings:**
   - **Alga URL**: `http://your-alga-host:8080` (must be reachable from the Mattermost server)
   - **Webhook Secret**: Same value as `MATTERMOST_WEBHOOK_SECRET`

::: warning Network reachability
The Mattermost server must be able to reach the Alga backend URL. If Alga is behind a reverse proxy or firewall, ensure the plugin can reach it. The Alga backend must also be able to reach `MATTERMOST_SERVER_URL`.
:::

## Channel Mapping

Route alerts to specific Mattermost channels using [routing rules](/core-features/routing):

1. Go to **Routes** in the Alga UI
2. Create a rule with conditions (e.g., `namespace = production`)
3. Set the destination to a Mattermost channel (e.g., `#prod-alerts`)

Alerts that don't match any routing rule go to `MATTERMOST_DEFAULT_CHANNEL`.

### Common Routing Patterns

| Pattern        | Example                                    |
| -------------- | ------------------------------------------ |
| By environment | `environment: production` → `#prod-alerts` |
| By severity    | `severity: critical` → `#critical-alerts`  |
| By team        | `team: payments` → `#payments-oncall`      |
| By service     | `service: api-gateway` → `#api-alerts`     |

## Bidirectional Sync

When the plugin is installed and configured:

- **Alert notifications** — Alga posts a threaded message with full alert details (labels, annotations, severity, fingerprint)
- **Thread replies** — messages posted in the Mattermost thread appear in the Alga investigation thread
- **Alga → Mattermost** — agent findings, status updates, and operator comments from Alga appear in the Mattermost thread
- **Acknowledgments** — operators can acknowledge alerts from either side

This means your team can collaborate on an alert entirely from Mattermost, and everything they say is preserved in the Alga investigation record.

## Bot Methods

The Mattermost integration exposes these internal methods:

| Method             | Description                                          |
| ------------------ | ---------------------------------------------------- |
| `CreatePost`       | Post a message to a channel                          |
| `ReplyToPost`      | Post a threaded reply under an existing post         |
| `GetChannelByName` | Resolve a channel by name within the configured team |
| `TestConnection`   | Verify the server URL and webhook secret are valid   |

## Webhook Inbound

Thread replies from Mattermost are delivered to Alga via the inbound webhook handler (`webhook/mattermost.go`). The Mattermost plugin forwards post events to Alga, authenticated by the shared `MATTERMOST_WEBHOOK_SECRET`.

## User-Level Binding

Individual users can link their personal Mattermost account to receive DM notifications. Go to **Profile → Connected Accounts** to link your Mattermost account. Once linked, [notification preferences](/on-call/notification-preferences) can route specific notification types to Mattermost DMs.

## Disabling

Disable Mattermost in either of two ways:

- **Integrations page** — toggle `provider_enabled` off in the Alga UI
- **YAML config** — set `mattermost_disabled: true` (note: this is not an env var)

When disabled, Alga stops posting to Mattermost immediately. Existing threads remain in Mattermost but are no longer synced.

## Troubleshooting

### Alerts not appearing in Mattermost

- Verify `MATTERMOST_SERVER_URL` is correct and reachable from the Alga backend
- Check that `MATTERMOST_TEAM` matches an existing team slug
- Verify the target channel exists in the configured team
- Check the routing rule conditions — unmatched alerts go to `MATTERMOST_DEFAULT_CHANNEL`
- Ensure the Mattermost plugin is enabled and configured with the correct Alga URL and webhook secret

### Thread replies not syncing back to Alga

- Verify the Mattermost plugin is installed and enabled (not just the env vars)
- Check that `MATTERMOST_WEBHOOK_SECRET` matches in both Alga and the plugin settings
- Ensure the Mattermost server can reach the Alga backend URL configured in the plugin
- Check Mattermost plugin logs for webhook delivery errors

### Plugin won't start

- Check Mattermost system console logs for plugin errors
- Verify the plugin version is compatible with your Mattermost Server version
- Ensure the Alga URL in plugin settings has no trailing slash and includes the scheme (`http://` or `https://`)

## See Also

- [Slack Integration](/integrations/slack) — the alternative chat platform integration
- [Routing](/core-features/routing) — configure which alerts go to which channels
- [Notifications](/core-features/notifications) — the notification dispatch system
- [Environment Variables](/configuration/environment-variables) — all Mattermost config vars
