---
title: System Configuration API
description: Runtime system configuration endpoints for configurable settings managed through the API.
---

# System Configuration API

Runtime settings can be managed via the System Configuration API, overriding environment variables for supported settings. Changes apply immediately without a restart and persist to the database.

## API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/system/config` | `system:read` | Get current runtime config |
| `PUT` | `/api/v1/system/config` | `system:write` | Update config fields |

## Get Configuration

```sh
curl -b cookies.txt http://localhost:8080/api/v1/system/config
```

Returns all configurable fields with their current values. Secrets (`google_client_secret`, `oidc_client_secret`) are never returned; instead, `google_client_secret_set` and `oidc_client_secret_set` booleans indicate whether a secret is configured. The response includes `updated_at` (RFC 3339) when config has been modified via PUT.

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

### Update Semantics

- Empty or omitted fields are left unchanged (partial update).
- If no recognized fields are set, the response is `"no changes"`.
- `log_level` re-initializes the logger immediately on change.
- Auth secrets (`google_client_secret`, `oidc_client_secret`) are encrypted via AES-256-GCM before database persistence. An empty string means "leave unchanged" since secrets are never returned on GET.
- Each successful PUT writes an audit log entry (`system_config_updated`).
- API overrides take precedence over environment variables and persist across restarts (loaded from DB on startup).

## Configurable Settings

### Core Pipeline

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `log_level` | string | `debug`, `info`, `warn`, `error`, `fatal` | Runtime log level; re-initializes logger immediately |
| `session_expiry_hours` | int | > 0, <= 720 | Session lifetime in hours |
| `max_concurrent_investigations` | int | > 0 | Per-agent investigation capacity |
| `correlation_window` | duration | Go duration string | Alert correlation time window |
| `correlation_cooldown_ttl` | duration | Go duration string | Cooldown after investigation publish |
| `investigation_timeout` | duration | Go duration string | Max investigation duration before timeout |

### Agent & Scheduler

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `agent_presence_ttl` | duration | Go duration string | Agent SSE presence TTL |
| `agent_disconnect_grace` | duration | Go duration string | Grace period after agent SSE disconnect |
| `scheduler_leader_ttl` | duration | Go duration string | Leader lease TTL for scheduler election |

### Slack Incident Channels

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `slack_incident_channels_enabled` | bool | — | Enable automatic Slack incident channels |
| `slack_incident_channel_visibility` | string | `public`, `private` | Channel visibility |
| `slack_incident_channel_trigger_status` | string | `active`, `detected` | Incident status that triggers channel creation |
| `slack_incident_channel_archive_on_close` | bool | — | Archive channel when incident closes |

### Incident Summaries

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `incident_summary_enabled` | bool | — | Enable periodic incident summary generation |
| `incident_summary_interval` | duration | Go duration string | Default summary cadence |
| `incident_summary_intervals` | map[severity]duration | Go duration string values | Per-severity cadence overrides (replaces all overrides when provided) |

### Authentication — Google OAuth

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `google_oauth_enabled` | bool | — | Enable Google Sign-In |
| `google_client_id` | string | — | Google OAuth client ID |
| `google_client_secret` | string | Non-empty to update | Google OAuth client secret (never returned on GET) |
| `google_oauth_redirect_url` | string | — | Override callback URL |

### Authentication — OIDC

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `oidc_enabled` | bool | — | Enable generic OIDC SSO |
| `oidc_issuer_url` | string | — | OIDC issuer discovery URL |
| `oidc_client_id` | string | — | OIDC client ID |
| `oidc_client_secret` | string | Non-empty to update | OIDC client secret (never returned on GET) |
| `oidc_scopes` | string | — | Space-separated OIDC scopes |

## Duration Format

All duration fields accept Go duration strings: `30s`, `5m`, `1h30m`, `24h`. Invalid durations return a `400` validation error.

## Settings Not Configurable via API

Settings not listed here (database URLs, `ENCRYPTION_KEYS`, `SECRET_PEPPER`, `RABBITMQ_URI`, `VALKEY_ADDR`, SMTP credentials) must be configured via environment variables and require a restart.

## See Also

- [Environment Variables](/configuration/environment-variables) — full env var reference
- [Security & Authentication](/configuration/security) — security configuration
