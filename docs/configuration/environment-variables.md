---
title: Environment Variables
description: Complete reference for every Alga environment variable — database, crypto, Valkey, RabbitMQ, agents, triage, OIDC, email, voice, and more.
---

# Configuration

Alga is configured via environment variables. The canonical reference is `apps/backend/.env.example` — copy it to `apps/backend/.env` and fill in the values. A YAML config file (`CONFIG_PATH`) is also supported for non-route settings; environment variables always override the YAML file.

`setup.sh` generates `.env` files with random secrets for local Docker Compose. Use it for a fast start:

```sh
./setup.sh && docker compose up -d
```

## General

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `PORT` | `8080` | No | Backend HTTP listen port |
| `LOG_LEVEL` | `info` | No | Log level: debug, info, warn, error |
| `LOG_FILE` | | No | Path to log file output |
| `LOG_FORMAT` | `text` | No | Log output format: `text` (human-readable) or `json` (structured) |
| `ENVIRONMENT` | | No | Set to `production` to enforce production checks (secure cookies). Crypto config is required in **all** environments |
| `CONFIG_PATH` | `./config/config.yaml` | No | Optional YAML config file path |
| `ALGA_BASE_URL` | | No | Base URL used for incident channel links |

## Admin Bootstrap

On first boot with no users in the database, the frontend redirects to `/setup` where the operator creates the initial admin account (email, password, and full name) via the setup wizard. The setup wizard is only available when no users exist — once an admin exists, it is disabled.

To reset a user's password later, use the CLI: `alga user reset-password <email>` (see [CLI reference](/operations/cli)).

## Session & Cookies

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SESSION_EXPIRY_HOURS` | `24` | No | Sliding (idle) session expiry |
| `SESSION_MAX_LIFETIME` | `12h` | No | Absolute max session lifetime (enforced in addition to idle expiry) |
| `SECURE_COOKIES` | `false` | No | Set Secure flag on cookies. Auto-enabled in production |

## HTTP Server Hardening

All have safe defaults; override only to tune for your deployment.

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_READ_HEADER_TIMEOUT` | `10s` | Max duration reading request headers |
| `SERVER_READ_TIMEOUT` | `30s` | Max duration reading the full request |
| `SERVER_WRITE_TIMEOUT` | `30s` | Max duration writing the response |
| `SERVER_IDLE_TIMEOUT` | `120s` | Max idle time for keep-alive connections |
| `SERVER_MAX_HEADER_BYTES` | `1048576` | Max request header size in bytes (1 MiB) |

## PostgreSQL

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `POSTGRES_DSN` | | Yes | PostgreSQL connection string (e.g. `postgres://user:pass@localhost:5432/alga?sslmode=disable`). Production must use `sslmode=require` or `verify-full` |
| `POSTGRES_AUTO_MIGRATE` | `false` | No | Run Ent auto-migration on startup (enabled in Docker Compose) |

## Cryptography

::: warning Required in every environment
`ENCRYPTION_KEYS` (or `ENCRYPTION_KEY`) **and** `SECRET_PEPPER` are required at startup in **all** environments, not only production.
:::

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `ENCRYPTION_KEYS` | | Yes | Comma-separated `kid:base64(32B)` pairs for AES-256-GCM envelope encryption. The highest `kid` seals new ciphertexts; lower kids decrypt historical data. Generate with `openssl rand -base64 32` |
| `ENCRYPTION_KEY` | | Yes | Single base64(32B) key, treated as `kid=1`. Ignored when `ENCRYPTION_KEYS` is set |
| `SECRET_PEPPER` | | Yes | HMAC pepper for password pre-hashing and token hashing. Generate with `openssl rand -base64 32` |

### Argon2id Tuning

Defaults follow OWASP 2026 (m=64 MiB, t=3, p=2). Tune so a single hash takes ~250–500ms on production hardware.

| Variable | Default | Description |
|----------|---------|-------------|
| `ARGON2_MEMORY_KIB` | `65536` | Memory cost in KiB (64 MiB) |
| `ARGON2_TIME` | `3` | Iterations |
| `ARGON2_PARALLELISM` | `2` | Parallelism factor |
| `ARGON2_SALT_LEN` | `16` | Salt length in bytes |
| `ARGON2_KEY_LEN` | `32` | Derived key length in bytes |
| `MAX_CONCURRENT_PASSWORD_HASHES` | `2 * NumCPU` | Max concurrent hash/verify ops (bounds API memory) |

### Trusted Proxies

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `TRUSTED_PROXIES` | | No | Comma-separated list of trusted proxy IPs/CIDRs for rate limiting and client-IP extraction |

## Valkey / Redis

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `VALKEY_ADDR` | | No | Valkey/Redis address (enables sessions, caching, rate limiting, leader election, presence, SLA, escalation state, on-call cache) |
| `VALKEY_PASSWORD` | | No | Valkey/Redis password |
| `VALKEY_DB` | `0` | No | Database number |

## RabbitMQ

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `RABBITMQ_URI` | | No | AMQP URI (enables async pipeline: alert processing, investigations, email, escalation, triage, SLA) |

## Slack

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SLACK_BOT_TOKEN` | | No | Slack bot token (UI-configurable, stored encrypted) |
| `SLACK_DEFAULT_CHANNEL` | | No | Default Slack channel for unmatched alerts |
| `SLACK_DISABLED` | `false` | No | Disable Slack delivery |
| `SLACK_SIGNING_SECRET` | | No | Verifies Slack Events API signatures on `/webhooks/slack` |
| `SLACK_CLIENT_ID` | | No | Slack app Client ID (enables OAuth install flow) |
| `SLACK_CLIENT_SECRET` | | No | Slack app Client Secret (enables OAuth install flow) |
| `SLACK_OAUTH_REDIRECT_URL` | | No | Override OAuth callback URL (for reverse-proxy setups) |

## Mattermost

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `MATTERMOST_SERVER_URL` | | No | Mattermost server base URL (Alga appends `/plugins/com.alga.mattermost-plugin` for plugin API) |
| `MATTERMOST_WEBHOOK_SECRET` | | No | Shared secret with the Mattermost plugin |
| `MATTERMOST_TEAM` | | No | Mattermost team slug for channel resolution (e.g., `engineering`) |
| `MATTERMOST_DEFAULT_CHANNEL` | | No | Default Mattermost channel for unmatched alerts |
| `MATTERMOST_DISABLED` | `false` | No | Disable Mattermost delivery. Only settable via YAML config or the Integrations API, **not** as an env var |

## Voice Escalation

Alga supports two voice providers for phone-call escalation: **Twilio** (default) and **Telnyx**. They are mutually exclusive — only the selected provider is active. Set `VOICE_PROVIDER` to choose; switching requires a restart. When unset, the provider can be chosen from the Integrations UI (persisted in the DB).

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `VOICE_PROVIDER` | `twilio` | No | `twilio` or `telnyx` |

### Twilio

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `TWILIO_ACCOUNT_SID` | | No | Twilio account SID. When set via env, the UI fields are locked and env takes precedence |
| `TWILIO_AUTH_TOKEN` | | No | Twilio auth token (required for callback validation) |
| `TWILIO_FROM_NUMBER` | | No | Twilio outbound phone number |
| `TWILIO_DISABLED` | `false` | No | Disable Twilio entirely |

### Telnyx

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `TELNYX_API_KEY` | | No | Telnyx API key. When set via env, the UI fields are locked |
| `TELNYX_CONNECTION_ID` | | No | Telnyx Call Control Application ID |
| `TELNYX_FROM_NUMBER` | | No | Telnyx outbound phone number |
| `TELNYX_PUBLIC_KEY` | | No | Ed25519 public key (base64) from the Telnyx portal, used to verify inbound call-control webhooks at `/api/v1/telnyx/callback`. Required when `VOICE_PROVIDER=telnyx` |
| `TELNYX_DISABLED` | `false` | No | Disable Telnyx entirely |
| `TELNYX_TTS_VOICE` | | No | TTS voice for spoken prompts (e.g. `Polly.Brian`, `Azure.en-CA-ClaraNeural`, `ElevenLabs.eleven_flash_v2_5.<voice_id>`) |
| `TELNYX_TTS_LANGUAGE` | | No | TTS language |
| `TELNYX_TTS_API_KEY_REF` | | No | Identifier of a Telnyx integration secret holding the ElevenLabs API key. Required only when `TELNYX_TTS_VOICE` starts with `ElevenLabs.` |

## SMTP / Email

Required for password reset emails and email notifications.

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SMTP_HOST` | | No | SMTP relay hostname (enables email notifications + password reset) |
| `SMTP_PORT` | `587` | No | SMTP port |
| `SMTP_USER` | | No | SMTP auth username |
| `SMTP_PASSWORD` | | No | SMTP auth password |
| `SMTP_FROM` | | No | From address for email notifications |
| `SMTP_SKIP_TLS_VERIFY` | `false` | No | Skip TLS certificate verification for SMTP. **Security risk** — logs a loud warning on startup when enabled |

## Hermes / SRE Agent

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `HERMES_PLATFORM_URL` | | No | Hermes platform base URL (stored in integrations table, encrypted token) |
| `HERMES_PLATFORM_TOKEN` | | No | Hermes platform bearer token (encrypted at rest) |

::: tip Agent connections use per-agent tokens
Agent dispatch and SSE connections use bearer tokens created per-agent in the Alga UI (`alga_agent_...`), not `HERMES_PLATFORM_TOKEN`. The platform URL/token fields are stored encrypted in the integrations table and surfaced via the integration config API.
:::

## Agent SSE

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `AGENT_SSE_ALLOWED_ORIGINS` | | No | Comma-separated `Origin` allowlist for the agent SSE endpoint |

## Alert Correlation

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `CORRELATION_WINDOW` | `0` (disabled) | No | Time window during which alerts sharing a correlation key are merged into one investigation |
| `CORRELATION_COOLDOWN_TTL` | `30m` | No | Cooldown after investigation publish (prevents duplicates) |

## Investigation

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `INVESTIGATION_TIMEOUT` | `10m` | No | Investigation timeout |
| `MAX_CONCURRENT_INVESTIGATIONS` | `3` | No | Max parallel investigations per agent |
| `INVESTIGATION_CHANNEL` | | No | Slack or Mattermost channel for investigation threads |
| `CRITICAL_SEVERITY_LABELS` | | No | Comma-separated labels that trigger critical escalation |

## Scheduler HA

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SCHEDULER_LEADER_TTL` | `15s` | No | Leader lease TTL. Set to `0` for single-replica (overridden to 15s when Valkey is present) |
| `AGENT_PRESENCE_TTL` | `90s` | No | Agent SSE presence TTL |
| `AGENT_DISCONNECT_GRACE` | `45s` | No | Grace period before resetting work on agent disconnect |

## Stale Alert Sweep

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `STALE_ALERT_THRESHOLD` | `15m` | No | Minimum age before a firing alert is considered stale. `0` disables |
| `STALE_ALERT_SWEEP_INTERVAL` | `5m` | No | How often the scheduler scans for stale alerts |

## Agent Memory

pgvector-backed shared agent memory for extracting and searching investigation memories.

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `MEMORY_ENABLED` | `false` | No | Enable agent memory |
| `MEMORY_EMBEDDING_URL` | | No | OpenAI-compatible embedding API URL |
| `MEMORY_EMBEDDING_API_KEY` | | No | API key for embedding service |
| `MEMORY_EMBEDDING_MODEL` | | No | Embedding model name |
| `MEMORY_LLM_URL` | | No | LLM API URL for memory extraction |
| `MEMORY_LLM_API_KEY` | | No | API key for extraction LLM |
| `MEMORY_LLM_MODEL` | | No | LLM model for memory extraction |
| `MEMORY_AUTO_EXTRACT` | `false` | No | Auto-extract memories on investigation completion |
| `MEMORY_MAX_PER_INVESTIGATION` | | No | Max memories extracted per investigation |
| `MEMORY_SIMILARITY_THRESHOLD` | | No | Min cosine similarity for search results |

## Triage

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `TRIAGE_ENABLED` | `false` | No | Enable the triage system |
| `TRIAGE_LLM_URL` | | No | LLM API URL for triage decisions |
| `TRIAGE_LLM_API_KEY` | | No | API key for triage LLM |
| `TRIAGE_LLM_MODEL` | | No | LLM model for triage |
| `TRIAGE_MAX_CONCURRENT` | `5` | No | Max concurrent triage operations |
| `TRIAGE_CONFIDENCE_THRESHOLD` | `0.7` | No | Min confidence for auto-decisions; below this, decisions downgrade to `enrich_only` |
| `TRIAGE_AUTO_RESOLVE_ENABLED` | `false` | No | Allow `auto_resolve` decisions (otherwise downgrades to `enrich_only`) |
| `TRIAGE_SUPPRESS_ENABLED` | `false` | No | Allow `suppress` decisions (otherwise downgrades to `enrich_only`) |
| `TRIAGE_CONTEXT_EPISODIC_LIMIT` | `5` | No | Max episodic context entries |
| `TRIAGE_CONTEXT_NOTES_LIMIT` | `3` | No | Max knowledge notes for context |
| `TRIAGE_CONTEXT_MEMORIES_LIMIT` | `3` | No | Max agent memories for context |
| `TRIAGE_AUTO_PROMOTE_CONFIRMED_COUNT` | `3` | No | Count of confirmed triages before auto-promotion |

## Google OAuth

Enables "Sign in with Google" on the login page.

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `GOOGLE_CLIENT_ID` | | No | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | | No | Google OAuth client secret |
| `GOOGLE_OAUTH_REDIRECT_URL` | | No | Override callback URL (auto-detected from request headers if not set) |
| `GOOGLE_OAUTH_ENABLED` | `true` | No | Toggle Google Sign-In |

## OIDC SSO

Single-provider quick config for OIDC SSO. Multiple providers can also be managed via **System → Authentication** (see [OIDC SSO](/integrations/oidc-sso)).

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `OIDC_ENABLED` | `false` | No | Enable OIDC SSO |
| `OIDC_ISSUER_URL` | | No | OIDC issuer URL |
| `OIDC_CLIENT_ID` | | No | OIDC client ID |
| `OIDC_CLIENT_SECRET` | | No | OIDC client secret |
| `OIDC_SCOPES` | `openid email profile` | No | OIDC scopes |

## On-Call

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `ON_CALL_REMIND_MINUTES` | `15` | No | Minutes before on-call shift to send reminder |
| `ON_CALL_REMIND_ENABLED` | `true` | No | Enable on-call shift reminders |

## Google Meet (War Rooms)

Auto-creates a Google Meet space per incident for war-room coordination. Requires a service-account JSON with the Meet API enabled and domain-wide delegation for scope `https://www.googleapis.com/auth/meet.space.admin`.

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `GOOGLE_MEET_ENABLED` | `false` | No | Enable Google Meet integration |
| `GOOGLE_MEET_CREDENTIALS_PATH` | | No | Path to the service-account JSON |
| `GOOGLE_MEET_AUTO_CREATE` | `true` | No | Auto-create a Meet space when an incident goes active |

## Data Retention

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `DATA_RETENTION_DAYS` | `90` | No | Days to retain resolved alerts. Set to `0` to keep forever |

## Features With No Environment Variables

These features are configured entirely through the API/UI, not environment variables:

- **Playbooks** — managed via `/api/v1/playbooks`
- **Heartbeats** — managed via `/api/v1/heartbeats`
- **Status Pages** — managed via `/api/v1/status-pages`
- **Credential Providers & Shared Secrets** — managed via `/api/v1/credential-providers` and `/api/v1/shared-secrets`

## System Configuration API

Runtime settings can also be managed via the API at `PUT /api/v1/system/config`. This overrides environment variables for supported settings. See [System Configuration API](/configuration/system-config).
