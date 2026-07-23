---
title: System Configuration API
description: Runtime system configuration endpoints for configurable settings managed through the API.
---

# System Configuration API

Runtime settings can be managed via the System Configuration API (`PUT /api/v1/system/config`), overriding environment variables for supported settings.

## API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/system/config` | `system:read` | Get runtime config |
| `PUT` | `/api/v1/system/config` | `system:write` | Update config |

## Get Configuration

```sh
curl -b cookies.txt http://localhost:8080/api/v1/system/config
```

Response includes current runtime settings with their sources (env var or API override).

## Update Configuration

```sh
curl -b cookies.txt -X PUT http://localhost:8080/api/v1/system/config \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: YOUR_CSRF_TOKEN" \
  -d '{
    "correlation_window": "5m",
    "investigation_timeout": "15m",
    "max_concurrent_investigations": 5
  }'
```

API overrides take precedence over environment variables. Changes apply immediately without restart.

## Configurable Settings

| Setting | Env Variable | Type | Description |
|---------|-------------|------|-------------|
| `correlation_window` | `CORRELATION_WINDOW` | duration | Alert correlation time window |
| `correlation_cooldown_ttl` | `CORRELATION_COOLDOWN_TTL` | duration | Cooldown after investigation publish |
| `investigation_timeout` | `INVESTIGATION_TIMEOUT` | duration | Max investigation duration |
| `max_concurrent_investigations` | `MAX_CONCURRENT_INVESTIGATIONS` | int | Per-agent capacity |
| `agent_presence_ttl` | `AGENT_PRESENCE_TTL` | duration | Agent SSE presence TTL |
| `agent_disconnect_grace` | `AGENT_DISCONNECT_GRACE` | duration | Grace period after SSE disconnect |
| `scheduler_leader_ttl` | `SCHEDULER_LEADER_TTL` | duration | Leader lease TTL |
| `session_expiry_hours` | `SESSION_EXPIRY_HOURS` | int | Session expiry in hours |
| `log_level` | `LOG_LEVEL` | string | Runtime log level (debug/info/warn/error/fatal) |
| `slack_incident_channels_enabled` | `SLACK_INCIDENT_CHANNELS_ENABLED` | bool | Enable Slack incident channels |
| `slack_incident_channel_visibility` | `SLACK_INCIDENT_CHANNEL_VISIBILITY` | string | Channel visibility (public/private) |
| `slack_incident_channel_trigger_status` | `SLACK_INCIDENT_CHANNEL_TRIGGER_STATUS` | string | Trigger status (active/open) |
| `slack_incident_channel_archive_on_close` | `SLACK_INCIDENT_CHANNEL_ARCHIVE_ON_CLOSE` | bool | Archive channel on incident close |

Settings not listed here (database URLs, secrets, credentials) must be configured via environment variables and require a restart.

## See Also

- [Environment Variables](/configuration/environment-variables) — full env var reference
- [Security & Authentication](/configuration/security) — security configuration
