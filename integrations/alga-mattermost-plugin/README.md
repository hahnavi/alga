# Alga Mattermost Plugin

A Mattermost server plugin that enables bidirectional sync between Mattermost and Alga. It forwards thread replies to Alga and exposes a REST API that Alga uses to post messages, manage threads, and list channels — all through the plugin, eliminating the need for a separate bot token.

## How It Works

The plugin serves two roles:

1. **Inbound (Mattermost → Alga)**: Intercepts thread replies via the `MessageHasBeenPosted` hook and forwards them as JSON payloads to the Alga webhook endpoint (`/webhooks/mattermost`).

2. **Outbound (Alga → Mattermost)**: Exposes REST API endpoints at `/plugins/com.alga.mattermost-plugin/api/v1/*` that Alga calls to create posts, reply to threads, update messages, resolve channels, and more. The plugin uses its internal bot user and Mattermost server API to execute these operations.

This means Alga only needs a single shared secret (the **Webhook Secret**) and the Mattermost server URL — no bot token or team ID required.

## Requirements

- Mattermost Server 7.0+
- Go 1.27+ (for building)
- Alga backend

## Installation

### From Pre-built Bundle

1. Download the latest `com.alga.mattermost-plugin-0.0.1.tar.gz` from releases.
2. Log in to Mattermost as a System Admin.
3. Go to **System Console > Plugins > Plugin Management**.
4. Upload the `.tar.gz` file.
5. Click **Enable**.

### From Source

```bash
cd integrations/mattermost-plugin
make tidy
make bundle
```

Then upload `dist/com.alga.mattermost-plugin-0.0.1.tar.gz` via the System Console.

### Manual Installation

```bash
make build
mkdir -p /path/to/mattermost/plugins/com.alga.mattermost-plugin
cp plugin.json /path/to/mattermost/plugins/com.alga.mattermost-plugin/
cp -r server/dist /path/to/mattermost/plugins/com.alga.mattermost-plugin/server/
# Restart Mattermost server
```

## Configuration

Configure the plugin in **System Console > Plugins > Alga Investigation Sync**.

| Setting              | Required | Description                                                                                                |
| -------------------- | -------- | ---------------------------------------------------------------------------------------------------------- |
| **Alga Webhook URL** | Yes      | The Alga backend webhook endpoint URL (e.g., `https://alga.example.com/webhooks/mattermost`)               |
| **Webhook Secret**   | Yes      | Shared secret for authenticating requests in both directions. Click **Regenerate** to create a new secret. |

On the Alga side, configure:

- `MATTERMOST_SERVER_URL` — The Mattermost server base URL (e.g., `https://mattermost.example.com`)
- `MATTERMOST_WEBHOOK_SECRET` — The same secret configured in the plugin

No bot token or team ID is needed on the Alga side — the plugin handles all Mattermost API interactions internally.

## Plugin REST API

The plugin exposes authenticated REST endpoints that Alga calls. All endpoints require `Authorization: Bearer <webhook_secret>`.

| Method | Path                          | Description                     |
| ------ | ----------------------------- | ------------------------------- |
| `GET`  | `/health`                     | Health check (no auth required) |
| `POST` | `/api/v1/post`                | Create a post in a channel      |
| `POST` | `/api/v1/reply`               | Reply to a thread               |
| `PUT`  | `/api/v1/update-post`         | Update an existing post         |
| `GET`  | `/api/v1/channel?name=<name>` | Resolve channel name to ID      |
| `GET`  | `/api/v1/channels`            | List all channels               |
| `GET`  | `/api/v1/username`            | Get bot username                |

### Request/Response Formats

#### POST /api/v1/post

```json
{
  "channel_id": "abc123",
  "message": "",
  "props": { "attachments": [...] }
}
```

Response: `{"id": "post_id"}`

#### POST /api/v1/reply

```json
{
  "root_post_id": "def456",
  "message": "Investigating...",
  "props": null
}
```

Response: `{"id": "reply_id", "channel_id": "abc123"}`

#### PUT /api/v1/update-post

```json
{
  "post_id": "ghi789",
  "message": "",
  "props": { "attachments": [...] }
}
```

Response: `{"status": "updated"}`

**GET /api/v1/channel?name=alerts**
Response: `{"id": "channel_id", "name": "alerts", "team_id": "team_id"}`

**GET /api/v1/channels**
Response: `[{"id": "...", "name": "..."}]`

**GET /api/v1/username**
Response: `{"username": "alga"}`

## Webhook Payload (Inbound)

The plugin sends the following JSON payload for each thread reply to the Alga webhook endpoint:

```json
{
  "post_id": "abc123",
  "root_id": "def456",
  "channel_id": "xyz789",
  "user_id": "user1",
  "user_name": "jdoe",
  "message": "Looking into this now",
  "team_id": "team1",
  "timestamp": 1712476800000,
  "event_type": "thread_reply"
}
```

The request includes:

- `Authorization: Bearer <webhook_secret>` header
- `Content-Type: application/json` header
- `User-Agent: Alga-Mattermost-Plugin/0.0.1` header

## Health Check

The plugin exposes a health endpoint at:

```
GET /plugins/com.alga.mattermost-plugin/health
```

Response:

```json
{ "status": "configured" }
```

Possible values for `status`:

- `configured` - The webhook URL is set and the plugin is ready.
- `not_configured` - The webhook URL has not been set.

## Development

```bash
# Install dependencies
make tidy

# Build for current platform
make build-dev

# Run tests
make test

# Format code
make fmt

# Lint
make lint

# Build all platforms and bundle
make bundle
```

## Architecture

```
Mattermost Server
├── MessageHasBeenPosted hook (inbound)
│   ├── Filter: only thread replies (RootId != "")
│   ├── Filter: skip bot user posts (prevent feedback loops)
│   ├── Filter: skip system posts (UserId == "")
│   └── Forward to Alga webhook (POST /webhooks/mattermost)
│       ├── JSON payload with post metadata
│       ├── Bearer token authentication
│       └── 10s request timeout
│
└── ServeHTTP endpoints (outbound, called by Alga)
    ├── POST /api/v1/post        → API.CreatePost()
    ├── POST /api/v1/reply       → API.CreatePost() with root_id
    ├── PUT /api/v1/update-post  → API.UpdatePost()
    ├── GET /api/v1/channel      → API.GetChannelByName()
    ├── GET /api/v1/channels     → API.GetChannelsForTeamForUser()
    └── GET /api/v1/username     → API.GetUser()
    All authenticated via shared webhook secret
```

## Security Considerations

- The webhook secret is marked as `secret` in the settings schema, preventing it from being exposed in API responses or the System Console after initial configuration.
- The same secret secures both inbound (plugin → Alga) and outbound (Alga → plugin) communication.
- Use HTTPS for the webhook URL to encrypt the payload in transit.
- The plugin filters out its own bot posts to prevent infinite feedback loops.
- Requests have a 10-second timeout to prevent blocking the Mattermost server on unresponsive backends.
- All plugin API endpoints validate the Bearer token before executing any operation.
