---
title: Slack OAuth Setup
description: Multi-workspace Slack OAuth 2.0 installation — token security, disconnect, scopes, and troubleshooting.
---

# Slack OAuth Setup

Alga supports OAuth 2.0 for Slack integration, enabling secure multi-workspace support.

## Prerequisites

- A Slack App with OAuth scopes configured
- Your Alga instance's public URL (for OAuth callbacks)
- App distribution enabled in your Slack App settings

## Configuration

Set these environment variables:

```sh
SLACK_CLIENT_ID=your-client-id
SLACK_CLIENT_SECRET=your-client-secret
SLACK_OAUTH_REDIRECT_URL=https://alga.yourdomain.com/api/v1/integrations/slack/oauth/callback
```

- `SLACK_CLIENT_ID` — From **Basic Information → App Credentials**
- `SLACK_CLIENT_SECRET` — From **Basic Information → App Credentials**
- `SLACK_OAUTH_REDIRECT_URL` — Optional; auto-detected from request headers. Override for reverse-proxy setups.

## OAuth Flow

### Step 1: Authorize

```bash
curl -X POST http://localhost:8080/api/v1/integrations/slack/oauth/authorize
```

Response:

```json
{
  "url": "https://slack.com/oauth/v2/authorize?client_id=xxx&scope=chat:write,channels:read&redirect_uri=..."
}
```

Redirect the user to `url`.

### Step 2: User Authorizes App

The user reviews permissions, selects a workspace, and clicks **Allow**.

### Step 3: Callback

Slack redirects back to Alga's callback. Alga automatically:

1. Exchanges the authorization code for a bot token
2. Stores the token encrypted at rest (AES-256-GCM)
3. Updates integration status to connected

### Step 4: Multi-Workspace

Each successful authorization adds a new workspace. `SLACK_BOT_TOKEN` env var is used as fallback when no OAuth tokens are configured.

## Token Security

- Bot tokens stored encrypted in the `integrations` table using AES-256-GCM
- Tokens never logged or exposed in API responses
- Token rotation happens on re-authorization
- Configure `ENCRYPTION_KEYS` in production for token encryption

## Disconnect a Workspace

```bash
curl -X POST http://localhost:8080/api/v1/integrations/slack/disconnect \
  -b cookies.txt
```

Removes OAuth tokens and disables Slack delivery. Re-connect by restarting the OAuth flow.

## OAuth Scopes

Configure in your Slack App's **OAuth & Permissions** section:

### Bot Token Scopes

The OAuth flow requests the following scopes:

| Scope                  | Purpose                                             |
| ---------------------- | --------------------------------------------------- |
| `chat:write`           | Send messages to channels                           |
| `chat:write.customize` | Send messages with a customized username and avatar |
| `chat:write.public`    | Post to public channels without joining             |
| `channels:read`        | Read channel information                            |
| `groups:read`          | Read private channels                               |
| `channels:manage`      | Create, archive, and manage channels                |
| `groups:manage`        | Create, archive, and manage private channels        |
| `channels:history`     | Read message history in channels                    |
| `groups:history`       | Read message history in private channels            |
| `im:write`             | Open and send direct messages                       |
| `mpim:write`           | Open and send group direct messages                 |

## Troubleshooting

### `redirect_uri_mismatch`

Verify `SLACK_OAUTH_REDIRECT_URL` matches your Slack App's redirect URL exactly. Ensure HTTPS in production.

### Bot token missing after OAuth

Verify bot scopes are configured and the bot user is enabled in the app. Review Alga logs for token storage errors.

### Multi-workspace not working

Ensure workspace-wide installation is enabled. Verify `ENCRYPTION_KEYS` is configured. Check the `integrations` table.

## See Also

- [Slack Integration](/integrations/slack) — general Slack setup and configuration
- [Configuration](/configuration/environment-variables) — environment variable reference
