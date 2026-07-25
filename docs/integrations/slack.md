---
title: Slack Integration
description: Full Slack workspace integration — bot setup, Events API, bidirectional thread sync, automated incident channels, and user-level binding.
---

# Slack Integration

Alga integrates with Slack for alert notifications, bidirectional thread sync, and automated incident channels.

## Configuration

### Required

```sh
SLACK_BOT_TOKEN=xoxb-your-bot-token
SLACK_SIGNING_SECRET=your-signing-secret
SLACK_DEFAULT_CHANNEL=#alerts
```

- `SLACK_BOT_TOKEN` — Bot user OAuth token (`xoxb-...`). Requires `chat:write`, `channels:read`, `groups:read` scopes.
- `SLACK_SIGNING_SECRET` — Signs Events API requests for verification.
- `SLACK_DEFAULT_CHANNEL` — Channel for unmatched alerts.

### Optional

```sh
SLACK_DISABLED=false
SLACK_CLIENT_ID=your-client-id
SLACK_CLIENT_SECRET=your-client-secret
SLACK_OAUTH_REDIRECT_URL=https://alga.yourdomain.com/api/v1/integrations/slack/oauth/callback
```

- `SLACK_DISABLED` — Set to `true` to disable Slack delivery even when configured.
- `SLACK_CLIENT_ID` — Slack app client ID (required for OAuth flows).
- `SLACK_CLIENT_SECRET` — Slack app client secret (required for OAuth flows).
- `SLACK_OAUTH_REDIRECT_URL` — OAuth redirect URL override for reverse-proxy setups.

### OAuth (Multi-Workspace)

For multi-workspace support via OAuth 2.0, see [Slack OAuth Setup](/integrations/slack-oauth).

## App Setup

### 1. Create a Slack App

1. Go to [api.slack.com/apps](https://api.slack.com/apps) → **Create New App** → **From manifest**
2. Use the manifest from `integrations/alga-slack-app/manifest.json` in the Alga repository (or run `integrations/alga-slack-app/setup.sh` for guided setup)
3. Install the app to your workspace
4. Copy the **Bot User OAuth Token** to `SLACK_BOT_TOKEN`

### 2. Configure Event Subscriptions

1. Enable **Event Subscriptions** in your app settings
2. Set the request URL to `http://your-alga-host:8080/webhooks/slack`
3. Subscribe to these events:
   - `message.channels` (for thread replies)

### 3. Configure Signing Secret

Copy the **Signing Secret** from **Basic Information** to `SLACK_SIGNING_SECRET`.

## Bot Token Scopes

Configure these scopes in your Slack App's **OAuth & Permissions** section:

| Scope | Purpose | Required |
|-------|---------|----------|
| `chat:write` | Send messages to channels | Yes |
| `chat:write.public` | Post to public channels without joining | Yes |
| `channels:join` | Join public channels | Recommended |
| `channels:read` | Read channel information | Yes |
| `groups:read` | Read private channels/DMs | Recommended |
| `im:write` | Send direct messages | Optional |
| `commands` | Receive slash commands | Optional |
| `app_mentions:read` | Read app mentions | No |

## Channel Types

| Type | Format | Bot Behavior |
|------|--------|-------------|
| Public channels | `#channel-name` | Can post without joining with `chat:write.public` |
| Private channels | Channel ID (`C0123ABCD`) | Bot must be invited first |
| Direct messages | User ID (`U0123ABCD`) | Bot can open DM with `im:write` scope |

## Message Formatting

Alga uses Slack Block Kit for rich alert messages with action buttons for acknowledge, resolve, and investigate.

### Threading Strategy

1. **Initial alert** — Posted as thread parent
2. **Comments** — Added as thread replies
3. **Investigation updates** — Threaded under the alert
4. **Agent responses** — Threaded for context preservation

## User-Level Slack Binding

Individual users can link their personal Slack account to receive DM notifications. This is separate from the workspace-level bot integration and allows per-user notification delivery.

### How It Works

1. The user clicks **Link Slack** in their notification preferences
2. Alga redirects to Slack's OAuth authorize URL
3. After approval, Slack redirects back to Alga with an authorization code
4. Alga exchanges the code for a user token and stores it
5. Notifications configured for Slack DM are sent directly to the user

### API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/users/me/slack/authorize` | Session | Initiate personal Slack account linking |
| `GET` | `/api/v1/users/me/slack/callback` | Session | OAuth callback for user-level Slack binding |
| `POST` | `/api/v1/users/me/slack/disconnect` | Session | Disconnect personal Slack account |

### Setup

1. Ensure the Slack app is configured with `im:write` scope
2. Add the user-level OAuth redirect URL to your Slack app's **OAuth & Permissions → Redirect URLs**
3. Users can link their account from **Profile → Notification Preferences → Slack DM**

## Slack Incident Channels

When enabled, Alga can automatically create dedicated Slack channels for incidents. Channels provide a focused space for incident coordination separate from general alert channels.

### Configuration

Configure via system config (`PUT /api/v1/system/config`):

| Setting | Values | Description |
|---------|--------|-------------|
| `slack_incident_channels_enabled` | `true`/`false` | Enable auto-creation |
| `slack_incident_channel_visibility` | `public`/`private` | Channel visibility |
| `slack_incident_channel_trigger_status` | `active`/`detected` | When to create the channel |
| `slack_incident_channel_archive_on_close` | `true`/`false` | Archive on incident close |

When `slack_incident_channels_enabled` is `true` and an incident reaches the trigger status, Alga automatically creates a channel named `inc-{incident-number}-{slug}` (for example `inc-1234-database-outage`) and posts the initial incident summary. If the name is already taken in Slack, Alga retries with `-2`, `-3` suffixes.

### API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/slack-channel` | `incidents:command` | Create Slack channel for incident |
| `DELETE` | `/api/v1/incidents/{id}/slack-channel` | `incidents:command` | Unlink Slack channel from incident |

Channels are automatically archived when incidents are closed (if configured).

## Channel Mapping

Route alerts to specific Slack channels using routing rules:

1. Go to **Routes** in Alga
2. Create a rule with conditions (provider, labels, severity)
3. Set the destination to a Slack channel (`#channel-name` or channel ID)

## Testing Your Integration

1. **Test connection:** Go to **Integrations** → **Slack** → **Test Connection**
2. **Send test alert:** Use the **Create Alert** dialog
3. **Verify delivery:** Check the target Slack channel
4. **Test threading:** Reply to the alert in Slack and verify it syncs to Alga

## Bot Methods

The Slack integration exposes these internal methods for sending messages and managing conversations:

| Method | Description |
|--------|-------------|
| `PostMessage` | Post a message to a channel (Block Kit formatted) |
| `PostThreadReply` | Post a threaded reply under an existing message |
| `OpenConversation` | Open a DM conversation with a user |
| `GetPermalink` | Get a permanent link to a message |
| `GetBotUserID` | Resolve the bot's own user ID |
| `TestConnection` | Verify the bot token is valid and the workspace is reachable |

## Slack Sign-In

Alga supports signing in with Slack as an authentication method (separate from the workspace-level bot integration and user-level OAuth binding):

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/auth/slack` | Public | Authenticate a user via Slack identity |

## API Reference

### Integration Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/integrations` | — | Get integration status |
| `PUT` | `/api/v1/integrations` | `integrations:write` | Update integration settings |
| `POST` | `/api/v1/integrations/test` | `integrations:test` | Test Slack connection |
| `POST` | `/api/v1/integrations/slack/oauth/authorize` | `integrations:write` | Initiate OAuth flow |
| `GET` | `/api/v1/integrations/slack/oauth/callback` | — | OAuth callback |
| `POST` | `/api/v1/integrations/slack/disconnect` | `integrations:write` | Disconnect workspace |

### Webhook Endpoint

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/webhooks/slack` | Signing secret | Slack Events API webhook |

## Troubleshooting

### Webhook Failures

1. Verify webhook URL is publicly accessible
2. Check `SLACK_SIGNING_SECRET` matches Slack app
3. Test via Slack's **Event Subscriptions** verification tool
4. Review Alga logs for signature validation errors

### Permission Errors

- Verify required scopes are granted (`chat:write`, `channels:read`)
- Invite bot to private channels before routing alerts there
- Use `chat:write.public` scope for easier public channel access

### Rate Limiting

- Slack has tier-based rate limits
- Alga implements automatic retries with exponential backoff
- Monitor `/debug/vars` for rate limit metrics

## Disabling

Set `SLACK_DISABLED=true` to disable Slack delivery while keeping configuration intact. Useful for maintenance windows or testing other integrations.
