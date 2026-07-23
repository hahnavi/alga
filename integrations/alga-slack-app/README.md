# Alga Slack App

A Slack app that provides bidirectional integration between Slack and Alga. It delivers rich Block Kit alert cards with interactive action buttons and syncs thread replies to investigations — all processed by the Alga backend.

## How It Works

The Slack integration runs entirely through the Alga backend — no separate service or proxy is needed. Slack communicates with Alga via HTTPS webhooks:

1. **Outbound (Alga → Slack):** When an alert fires, Alga posts a rich Block Kit card to the configured Slack channel via the Web API (`chat.postMessage`). The card includes alert details, labels, annotations, and interactive action buttons (Acknowledge, Resolve). When an alert is resolved, the card is updated in-place with a green status.

2. **Inbound — Events (Slack → Alga):** Slack sends `event_callback` payloads for `message.channels` and `message.groups` events to `/webhooks/slack`. Thread replies (`thread_ts` set) are matched to an investigation via `slack_channel_id` + `slack_thread_ts` on the investigation record. User messages in the thread are forwarded to the default SRE agent via SSE.

3. **Inbound — Interactive (Slack → Alga):** When a user clicks an action button (Acknowledge / Resolve), Slack sends a `block_actions` payload to `/webhooks/slack`. Alga processes the action, updates the alert, refreshes the Slack message (removing buttons, showing "Acknowledged/Resolved by @user"), publishes SSE events, and logs an audit entry.

This means Alga only needs a Bot OAuth Token and a Signing Secret — no additional infrastructure.

## Requirements

- Alga backend (v1.0.0+)
- A Slack workspace with permission to create apps
- Publicly reachable Alga endpoint (for Slack Events API / interactivity webhooks)

## Installation

### Quick Setup (Manifest Import)

1. Replace `YOUR_ALGA_HOST` in `manifest.json` with your Alga instance URL:
   ```bash
   sed -i 's|YOUR_ALGA_HOST|alga.example.com|g' manifest.json
   ```

2. Go to [api.slack.com/apps](https://api.slack.com/apps) and click **Create New App** → **From a manifest**.

3. Choose your workspace and paste the contents of `manifest.json`.

4. Review the app configuration and click **Create**.

5. Under **OAuth & Permissions**, click **Install to Workspace** and approve the permissions.

6. Copy the **Bot User OAuth Token** (starts with `xoxb-`).

7. Under **Basic Information → App Credentials**, copy the **Signing Secret**.

8. Configure Alga with the copied values (see Configuration below).

### Automated Setup

```bash
cd integrations/slack-app

# Validate manifest and show setup instructions
./setup.sh --manual

# Or create the app programmatically (requires a Slack admin token)
export SLACK_TOKEN="xoxp-..."
./setup.sh --interactive
```

## Configuration

### Alga Side

| Variable | Required | Description |
|---|---|---|
| `SLACK_BOT_TOKEN` | Yes | Bot User OAuth Token (`xoxb-...`). Used for posting messages, threads, and updates. |
| `SLACK_SIGNING_SECRET` | Yes | App Signing Secret. Verifies `X-Slack-Signature` on all inbound requests. |
| `SLACK_DEFAULT_CHANNEL` | No | Default Slack channel for unmatched alerts (e.g., `C0123456789` or `#alerts`). |
| `SLACK_DISABLED` | No | Set to `true` to disable Slack delivery even when a token is configured. |

### Slack Side

Configure in the app settings at api.slack.com:

| Setting | Value |
|---|---|
| **Event Subscriptions → Request URL** | `https://<alga-host>/webhooks/slack` |
| **Interactivity → Request URL** | `https://<alga-host>/webhooks/slack` |

### Bot OAuth Scopes

The manifest requests these scopes:

| Scope | Purpose |
|---|---|
| `chat:write` | Post alert cards, thread replies, and update messages |
| `chat:write.customize` | Post Alga-authored investigation replies with the linked Slack user's display name |
| `chat:write.public` | Post to public channels the bot has not been explicitly invited to (required for dynamic alert routing) |
| `channels:read` | List public channels for routing and channel resolution |
| `groups:read` | List private channels for routing and channel resolution |
| `channels:manage` | Create, archive, set topic/purpose, and invite users in public channels |
| `groups:manage` | Create, archive, set topic/purpose, and invite users in private channels |
| `channels:history` | Receive message events in public channels |
| `groups:history` | Receive message events in private channels |
| `im:write` | Open DM conversations for direct notifications |
| `mpim:write` | Open multi-party IM conversations for group notifications |

The `identity.basic` user token scope is also requested for user-to-Slack account linking (users can link their Slack identity from the Alga settings page).

## Features

### Rich Alert Cards (Block Kit)

Alerts are delivered as rich Block Kit messages with:

- **Status header**: Firing, Acknowledged, or Resolved
- **Structured fields**: Labels, annotations, values, and timestamps in a grid layout
- **Action buttons**: "Acknowledge" and "Resolve" (removed after action)
- **Footer context**: Links to Grafana, Silence, and Runbook with fingerprint

Example alert card structure:

```
┌─────────────────────────────────────┐
│ *Firing*  HighMemoryUsage           │
├─────────────────────────────────────┤
│ Memory usage exceeded 90% threshold │
├─────────────────────────────────────┤
│ *namespace:* production |           │
│ *severity:* critical    |           │
│ *pod:* web-frontend-7x9k |         │
│ *Started:* 2026-04-25 14:30 UTC    │
├─────────────────────────────────────┤
│ [Acknowledge]  [Resolve]            │
├─────────────────────────────────────┤
│ Grafana · Silence · FP: abc123      │
└─────────────────────────────────────┘
```

### Interactive Action Buttons

Users can click **Acknowledge** or **Resolve** directly in Slack:

- The button triggers a `block_actions` payload to Alga
- Alga updates the alert state in the database
- The Slack message is updated: buttons are removed and replaced with a status line (e.g., "_Acknowledged by @jdoe_")
- An SSE event is published to all connected frontends
- An audit log entry is recorded

### Bidirectional Thread Sync

Thread replies in Slack are synced to Alga investigations:

1. When an alert is delivered to Slack, the `channel_id` and message `ts` are stored on the investigation record.
2. Slack sends `message` events for thread replies to `/webhooks/slack`.
3. Alga matches the reply to an investigation via `slack_channel_id` + `slack_thread_ts`.
4. The reply is added as an investigation update with `source: "slack"`.
5. The message is forwarded to the assigned SRE agent via SSE (unless it's a bot message or starts with `🔒` for internal notes).

## Investigation Matching

Investigations store:

- `slack_channel_id` — Slack channel ID (e.g. `C012345`)
- `slack_thread_ts` — Root message `ts` of the alert thread

Replies must be **in-thread** (`thread_ts` present). Top-level channel messages are ignored.

## Security

- **Request verification**: All inbound requests (Events API, interactivity) are verified using the Slack signing secret (`X-Slack-Signature` + `X-Slack-Request-Timestamp` HMAC-SHA256 with 5-minute replay window). Signature verification is performed on the raw request body before any parsing.
- **Bot dedup**: Bot messages are filtered by both `bot_id`/`subtype` and a registered bot user ID to prevent feedback loops.
- **No secrets stored by the app**: The Slack app itself doesn't store any Alga credentials — all auth is one-directional (Slack → Alga via signing secret, Alga → Slack via bot token).
- **HTTPS required**: The request URL must use HTTPS in production.
- **Token rotation**: The manifest enables Slack token rotation for enhanced security.

## Message Lifecycle

```
Alert fires → Alga routes to Slack channel
            → Posts Block Kit card with [Acknowledge] [Resolve]
            → Stores channel_id + ts on investigation

User clicks [Acknowledge] → Slack sends block_actions to Alga
                          → Alga calls alertStore.AcknowledgeAlert()
                          → Updates Slack message (removes buttons)
                          → Publishes SSE event to frontends
                          → Logs audit entry

Thread reply in Slack → Slack sends event_callback to Alga
                      → Alga matches investigation by channel+thread
                      → Adds investigation update (source: slack)
                      → Forwards to SRE agent via SSE

Alert resolves (Grafana) → Alga updates all delivery targets
                         → Slack card updated to green "Resolved" state
                         → Buttons removed
```

## Related

- Mattermost plugin (similar bidirectional pattern): [`integrations/alga-mattermost-plugin/README.md`](../alga-mattermost-plugin/README.md)
- Backend Slack webhook handler: [`apps/backend/webhook/slack.go`](../../apps/backend/webhook/slack.go)
- Backend Slack client: [`apps/backend/slack/client.go`](../../apps/backend/slack/client.go)
- Backend Block Kit builder: [`apps/backend/slack/blocks.go`](../../apps/backend/slack/blocks.go)
- Backend alert delivery: [`apps/backend/webhook/delivery_posts.go`](../../apps/backend/webhook/delivery_posts.go)
