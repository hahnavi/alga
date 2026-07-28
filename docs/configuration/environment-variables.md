---
title: Environment Variables
description: Complete reference for every Alga environment variable — database, crypto, Valkey, RabbitMQ, agents, triage, email, voice, telemetry, and more.
---

# Configuration

Alga is configured via environment variables. The canonical reference is `apps/backend/.env.example` — copy it to `apps/backend/.env` and fill in the values. A YAML config file (`CONFIG_PATH`) is also supported for non-route settings; environment variables always override the YAML file, and the YAML file overrides built-in defaults.

`setup.sh` generates `.env` files with random secrets for local Docker Compose. Use it for a fast start:

```sh
./setup.sh && docker compose up -d
```

The default values below come from `Defaults()` in `apps/backend/config/config.go`. Duration values use Go duration syntax (`30s`, `10m`, `12h`, `168h`). Boolean values accept `true`/`false` (and other `strconv.ParseBool` forms).

## General

| Variable        | Default                | Required | Description                                                                                                                                           |
| --------------- | ---------------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PORT`          | `8080`                 | No       | Backend HTTP listen port                                                                                                                              |
| `LOG_LEVEL`     | `info`                 | No       | Log level: `debug`, `info`, `warn`, `error`                                                                                                           |
| `LOG_FILE`      |                        | No       | Path to log file output. Empty logs to stdout                                                                                                         |
| `LOG_FORMAT`    | `text`                 | No       | Log output format: `text` (human-readable) or `json` (structured)                                                                                     |
| `ENVIRONMENT`   |                        | No       | Set to `production` (or `prod`) to enforce production checks (secure cookies). Crypto config is required in **all** environments, not only production |
| `CONFIG_PATH`   | `./config/config.yaml` | No       | Optional YAML config file path for non-route settings                                                                                                 |
| `ALGA_BASE_URL` |                        | No       | Base URL used for incident channel links and outbound references                                                                                      |

## Admin Bootstrap

On first boot with no users in the database, the frontend redirects to `/setup` where the operator creates the initial admin account (email, password, and full name) via the setup wizard. The setup wizard is only available when no users exist — once an admin exists, it is disabled.

To reset a user's password later, use the CLI: `alga user reset-password <email>` (see [CLI reference](/operations/cli)).

## Session & Cookies

| Variable               | Default | Required | Description                                                                                                          |
| ---------------------- | ------- | -------- | -------------------------------------------------------------------------------------------------------------------- |
| `SESSION_EXPIRY_HOURS` | `24`    | No       | Sliding (idle) session expiry, in hours                                                                              |
| `SESSION_MAX_LIFETIME` | `12h`   | No       | Absolute max session lifetime, enforced in addition to idle expiry (ASVS V3.2/V3.3)                                  |
| `SECURE_COOKIES`       | `false` | No       | Set the `Secure` flag on cookies. Auto-enabled when `ENVIRONMENT=production` and HSTS is emitted on HTTPS regardless |

## HTTP Server Hardening

All have safe defaults; override only to tune for your deployment.

| Variable                     | Default   | Description                                                                    |
| ---------------------------- | --------- | ------------------------------------------------------------------------------ |
| `SERVER_READ_HEADER_TIMEOUT` | `10s`     | Max duration reading request headers (neutralizes slowloris)                   |
| `SERVER_READ_TIMEOUT`        | `30s`     | Max duration reading the full request (headers + body)                         |
| `SERVER_WRITE_TIMEOUT`       | `30s`     | Max duration writing the response. SSE handlers clear their own write deadline |
| `SERVER_IDLE_TIMEOUT`        | `120s`    | Max idle time for keep-alive connections                                       |
| `SERVER_MAX_HEADER_BYTES`    | `1048576` | Max request header size in bytes (1 MiB)                                       |

## PostgreSQL

| Variable                | Default | Required | Description                                                                                                                                            |
| ----------------------- | ------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `POSTGRES_DSN`          |         | Yes      | PostgreSQL connection string (e.g. `postgres://user:pass@localhost:5432/alga?sslmode=disable`). Production must use `sslmode=require` or `verify-full` |
| `POSTGRES_AUTO_MIGRATE` | `false` | No       | Run Ent auto-migration on startup (enabled in Docker Compose)                                                                                          |

## Cryptography

::: warning Fail-closed — required in every environment
`ENCRYPTION_KEYS` **and** `SECRET_PEPPER` are required at startup in **all** environments, not only production. If either is missing or invalid, the server refuses to start (ASVS V2.1/V6.2).
:::

| Variable          | Default | Required | Description                                                                                                                                                                                                                           |
| ----------------- | ------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ENCRYPTION_KEYS` |         | Yes      | Comma-separated `kid:base64(32B)` pairs for AES-256-GCM envelope encryption. The highest `kid` seals new ciphertexts; lower kids decrypt historical data. Generate keys with `openssl rand -base64 32` (e.g. `1:<base64>,2:<base64>`) |
| `SECRET_PEPPER`   |         | Yes      | Base64-encoded HMAC pepper (must decode to >= 32 bytes) for password pre-hashing and token hashing. Generate with `openssl rand -base64 32`                                                                                           |

::: tip `setup.sh` and `ENCRYPTION_KEY`
`setup.sh` appends a single `ENCRYPTION_KEY` value to `apps/backend/.env` for local convenience. The backend keyring reads `ENCRYPTION_KEYS` (the `kid:base64` form above); for any real deployment set `ENCRYPTION_KEYS` explicitly.
:::

### Argon2id Tuning

Defaults follow OWASP 2026 (m=64 MiB, t=3, p=2). Tune so a single hash takes ~250–500ms on production hardware. Memory is the dominant cost; combined with `MAX_CONCURRENT_PASSWORD_HASHES` it bounds API memory at roughly `ARGON2_MEMORY_KIB * MAX_CONCURRENT_PASSWORD_HASHES` KiB.

| Variable                         | Default      | Description                                               |
| -------------------------------- | ------------ | --------------------------------------------------------- |
| `ARGON2_MEMORY_KIB`              | `65536`      | Memory cost in KiB (64 MiB). Minimum accepted: 8192       |
| `ARGON2_TIME`                    | `3`          | Iterations. Minimum accepted: 1                           |
| `ARGON2_PARALLELISM`             | `2`          | Parallelism factor. Minimum accepted: 1                   |
| `ARGON2_SALT_LEN`                | `16`         | Salt length in bytes. Minimum accepted: 8                 |
| `ARGON2_KEY_LEN`                 | `32`         | Derived key length in bytes. Minimum accepted: 16         |
| `MAX_CONCURRENT_PASSWORD_HASHES` | `2 * NumCPU` | Max concurrent hash/verify operations (bounds API memory) |

### Trusted Proxies

| Variable          | Default | Required | Description                                                                                |
| ----------------- | ------- | -------- | ------------------------------------------------------------------------------------------ |
| `TRUSTED_PROXIES` |         | No       | Comma-separated list of trusted proxy IPs/CIDRs for rate limiting and client-IP extraction |

## Slack

| Variable                   | Default | Required | Description                                                                                     |
| -------------------------- | ------- | -------- | ----------------------------------------------------------------------------------------------- |
| `SLACK_BOT_TOKEN`          |         | No       | Slack bot token (UI-configurable, stored encrypted). When set via env, the UI fields are locked |
| `SLACK_DEFAULT_CHANNEL`    |         | No       | Default Slack channel for unmatched alerts                                                      |
| `SLACK_DISABLED`           | `false` | No       | Disable Slack delivery                                                                          |
| `SLACK_SIGNING_SECRET`     |         | No       | Verifies Slack Events API signatures on `/webhooks/slack`                                       |
| `SLACK_CLIENT_ID`          |         | No       | Slack app Client ID (enables OAuth install flow)                                                |
| `SLACK_CLIENT_SECRET`      |         | No       | Slack app Client Secret (enables OAuth install flow)                                            |
| `SLACK_OAUTH_REDIRECT_URL` |         | No       | Override OAuth callback URL (for reverse-proxy setups)                                          |

## Mattermost

| Variable                     | Default | Required | Description                                                                                                                                  |
| ---------------------------- | ------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `MATTERMOST_SERVER_URL`      |         | No       | Mattermost server base URL (Alga appends `/plugins/com.alga.mattermost-plugin` for the plugin API). When set via env, the UI field is locked |
| `MATTERMOST_WEBHOOK_SECRET`  |         | No       | Shared secret with the Mattermost plugin                                                                                                     |
| `MATTERMOST_TEAM`            |         | No       | Mattermost team slug for channel resolution (e.g. `engineering`)                                                                             |
| `MATTERMOST_DEFAULT_CHANNEL` |         | No       | Default Mattermost channel for unmatched alerts                                                                                              |
| `MATTERMOST_DISABLED`        | `false` | No       | Disable Mattermost delivery                                                                                                                  |

## RabbitMQ

| Variable       | Default | Required | Description                                                                                             |
| -------------- | ------- | -------- | ------------------------------------------------------------------------------------------------------- |
| `RABBITMQ_URI` |         | No       | AMQP URI (enables the async pipeline: alert processing, investigations, email, escalation, triage, SLA) |

## Valkey / Redis

| Variable          | Default | Required | Description                                                                                                                                          |
| ----------------- | ------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VALKEY_ADDR`     |         | No       | Valkey/Redis address (enables sessions, caching, rate limiting, leader election, presence, SLA, escalation state, on-call cache, idempotency replay) |
| `VALKEY_PASSWORD` |         | No       | Valkey/Redis password                                                                                                                                |
| `VALKEY_DB`       | `0`     | No       | Database number                                                                                                                                      |

## Idempotency & Outbox

| Variable                | Default         | Required | Description                                                                                                                                                                                                   |
| ----------------------- | --------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `IDEMPOTENCY_ENABLED`   | `true`          | No       | Enable `Idempotency-Key` replay for opted-in retry-safe writes (create incident, ingest alert webhook, notification send, agent state-changing actions). Requires Valkey; no-op when Valkey is not configured |
| `IDEMPOTENCY_TTL`       | `24h`           | No       | How long a cached response is replayed for a repeated `Idempotency-Key` before the key expires                                                                                                                |
| `ALGA_OUTBOX_RETENTION` | `168h` (7 days) | No       | Retention window for published transactional-outbox rows before pruning. Set to `0` to disable pruning                                                                                                        |

## Hermes / SRE Agent

| Variable                             | Default | Required | Description                                                                  |
| ------------------------------------ | ------- | -------- | ---------------------------------------------------------------------------- |
| `HERMES_PLATFORM_URL`                |         | No       | Hermes platform base URL (stored in the integrations table, encrypted token) |
| `HERMES_PLATFORM_TOKEN`              |         | No       | Hermes platform bearer token (encrypted at rest)                             |
| `HERMES_DELEGATION_POLL_INTERVAL`    | `3s`    | No       | Delegation polling interval (declared in `apps/backend/.env.example`)        |
| `HERMES_DELEGATION_WAIT_PER_ATTEMPT` | `2m`    | No       | Wait per delegation attempt (declared in `apps/backend/.env.example`)        |
| `HERMES_DELEGATION_MAX_ATTEMPTS`     | `30`    | No       | Max delegation attempts (declared in `apps/backend/.env.example`)            |

::: tip Agent connections use per-agent tokens
Agent dispatch and SSE connections use bearer tokens created per-agent in the Alga UI (`alga_agent_...`), not `HERMES_PLATFORM_TOKEN`. The platform URL/token fields are stored encrypted in the integrations table and surfaced via the integration config API.
:::

## Agent SSE

| Variable                    | Default | Required | Description                                                                             |
| --------------------------- | ------- | -------- | --------------------------------------------------------------------------------------- |
| `AGENT_SSE_ALLOWED_ORIGINS` |         | No       | Comma-separated `Origin` allowlist for the agent SSE endpoint. Empty allows all origins |

## Alert Correlation

| Variable                   | Default        | Required | Description                                                                                               |
| -------------------------- | -------------- | -------- | --------------------------------------------------------------------------------------------------------- |
| `CORRELATION_WINDOW`       | `0` (disabled) | No       | Time window during which alerts sharing a correlation key are merged into one investigation. `0` disables |
| `CORRELATION_COOLDOWN_TTL` | `30m`          | No       | Cooldown after investigation publish (prevents duplicates)                                                |

## Investigation

| Variable                        | Default | Required | Description                                             |
| ------------------------------- | ------- | -------- | ------------------------------------------------------- |
| `INVESTIGATION_TIMEOUT`         | `10m`   | No       | Investigation timeout                                   |
| `MAX_CONCURRENT_INVESTIGATIONS` | `3`     | No       | Max parallel investigations per agent                   |
| `MAX_CONCURRENT_COMMAND`        | `3`     | No       | Max concurrent command dispatches                       |
| `STATUS_UPDATE_INTERVAL`        | `15m`   | No       | Default cadence for incident status updates             |
| `INVESTIGATION_CHANNEL`         |         | No       | Slack or Mattermost channel for investigation threads   |
| `CRITICAL_SEVERITY_LABELS`      |         | No       | Comma-separated labels that trigger critical escalation |

## Scheduler HA & Presence

| Variable                 | Default         | Required | Description                                                                                                                                           |
| ------------------------ | --------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `SCHEDULER_LEADER_TTL`   | `15s`           | No       | Leader lease TTL for scheduler HA. Set to `0` for single-replica                                                                                      |
| `AGENT_PRESENCE_TTL`     | `90s`           | No       | Agent SSE presence TTL (renewed via heartbeat/keepalive)                                                                                              |
| `AGENT_DISCONNECT_GRACE` | `45s`           | No       | Grace period before resetting work on agent disconnect                                                                                                |
| `CANCEL_SET_TTL`         | `168h` (7 days) | No       | How long a deleted entity's cancel marker lives in Valkey so workers skip it without hitting Postgres. Must exceed the max RabbitMQ retry/DLQ horizon |

## Stale Alert Sweep

| Variable                     | Default | Required | Description                                                                               |
| ---------------------------- | ------- | -------- | ----------------------------------------------------------------------------------------- |
| `STALE_ALERT_THRESHOLD`      | `15m`   | No       | Minimum age before a firing alert with no investigation is considered stale. `0` disables |
| `STALE_ALERT_SWEEP_INTERVAL` | `5m`    | No       | How often the scheduler scans for stale alerts                                            |

## Stuck-Investigation Escalation

| Variable                                       | Default    | Required | Description                                                                                                                           |
| ---------------------------------------------- | ---------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `STUCK_INVESTIGATION_ESCALATION_MULTIPLIER`    | `2`        | No       | Multiple of `INVESTIGATION_TIMEOUT` after which an in-progress investigation is "stuck" and pages the ops team. Set to `0` to disable |
| `STUCK_INVESTIGATION_ESCALATION_TICK_INTERVAL` | `30s`      | No       | How often the stuck-investigation escalation worker runs                                                                              |
| `OPS_TEAM_NAME`                                | `ops-team` | No       | Team whose schedule is the human-on-call target for stuck-investigation and forced-channel escalations                                |

## Voice Escalation

Alga supports two voice providers for phone-call escalation: **Twilio** (default) and **Telnyx**. They are mutually exclusive — only the selected provider is active. Set `VOICE_PROVIDER` to choose; switching requires a restart. When unset, the provider can be chosen from the Integrations UI (persisted in the DB).

| Variable         | Default  | Required | Description                                                                                         |
| ---------------- | -------- | -------- | --------------------------------------------------------------------------------------------------- |
| `VOICE_PROVIDER` | `twilio` | No       | `twilio` or `telnyx`. Unrecognized values normalize to `twilio`. Setting this locks the UI selector |

### Twilio

| Variable             | Default | Required | Description                                                                             |
| -------------------- | ------- | -------- | --------------------------------------------------------------------------------------- |
| `TWILIO_ACCOUNT_SID` |         | No       | Twilio account SID. When set via env, the UI fields are locked and env takes precedence |
| `TWILIO_AUTH_TOKEN`  |         | No       | Twilio auth token (required for callback validation)                                    |
| `TWILIO_FROM_NUMBER` |         | No       | Twilio outbound phone number                                                            |
| `TWILIO_TO_NUMBERS`  |         | No       | Comma-separated list of destination phone numbers                                       |
| `TWILIO_DISABLED`    | `false` | No       | Disable Twilio entirely                                                                 |

### Telnyx

| Variable                 | Default | Required | Description                                                                                                                                                          |
| ------------------------ | ------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TELNYX_API_KEY`         |         | No       | Telnyx API key. When set via env, the UI fields are locked                                                                                                           |
| `TELNYX_CONNECTION_ID`   |         | No       | Telnyx Call Control Application ID                                                                                                                                   |
| `TELNYX_FROM_NUMBER`     |         | No       | Telnyx outbound phone number                                                                                                                                         |
| `TELNYX_PUBLIC_KEY`      |         | No       | Ed25519 public key (base64) from the Telnyx portal, used to verify inbound call-control webhooks at `/api/v1/telnyx/callback`. Required when `VOICE_PROVIDER=telnyx` |
| `TELNYX_DISABLED`        | `false` | No       | Disable Telnyx entirely                                                                                                                                              |
| `TELNYX_TTS_VOICE`       |         | No       | TTS voice for spoken prompts (e.g. `Polly.Brian`, `Azure.en-CA-ClaraNeural`, `ElevenLabs.eleven_flash_v2_5.<voice_id>`). Defaults to a free Telnyx voice             |
| `TELNYX_TTS_LANGUAGE`    |         | No       | TTS language                                                                                                                                                         |
| `TELNYX_TTS_API_KEY_REF` |         | No       | Identifier of a Telnyx integration secret holding the ElevenLabs API key. Required only when `TELNYX_TTS_VOICE` starts with `ElevenLabs.`                            |

## SMTP / Email

Required for password reset emails and email notifications. If `SMTP_HOST` is empty, password reset emails are logged but not delivered.

| Variable               | Default | Required | Description                                                                                                             |
| ---------------------- | ------- | -------- | ----------------------------------------------------------------------------------------------------------------------- |
| `SMTP_HOST`            |         | No       | SMTP relay hostname (enables email notifications + password reset)                                                      |
| `SMTP_PORT`            | `587`   | No       | SMTP port                                                                                                               |
| `SMTP_USER`            |         | No       | SMTP auth username                                                                                                      |
| `SMTP_PASSWORD`        |         | No       | SMTP auth password                                                                                                      |
| `SMTP_FROM`            |         | No       | From address for email notifications                                                                                    |
| `SMTP_SKIP_TLS_VERIFY` | `false` | No       | Skip TLS certificate verification for SMTP. **Security risk** — logs a loud warning on startup when enabled (ASVS V9.2) |

## Agent Memory

pgvector-backed shared agent memory for extracting and searching investigation memories.

| Variable                       | Default | Required | Description                                         |
| ------------------------------ | ------- | -------- | --------------------------------------------------- |
| `MEMORY_ENABLED`               | `false` | No       | Enable agent memory                                 |
| `MEMORY_EMBEDDING_URL`         |         | No       | OpenAI-compatible embedding API URL                 |
| `MEMORY_EMBEDDING_API_KEY`     |         | No       | API key for the embedding service                   |
| `MEMORY_EMBEDDING_MODEL`       |         | No       | Embedding model name                                |
| `MEMORY_LLM_URL`               |         | No       | OpenAI-compatible LLM API URL for memory extraction |
| `MEMORY_LLM_API_KEY`           |         | No       | API key for the extraction LLM                      |
| `MEMORY_LLM_MODEL`             |         | No       | LLM model for memory extraction                     |
| `MEMORY_AUTO_EXTRACT`          | `false` | No       | Auto-extract memories on investigation completion   |
| `MEMORY_MAX_PER_INVESTIGATION` |         | No       | Max memories extracted per investigation            |
| `MEMORY_SIMILARITY_THRESHOLD`  |         | No       | Min cosine similarity (0–1) for search results      |

## Triage

| Variable                              | Default | Required | Description                                                                         |
| ------------------------------------- | ------- | -------- | ----------------------------------------------------------------------------------- |
| `TRIAGE_ENABLED`                      | `false` | No       | Enable the triage system                                                            |
| `TRIAGE_LLM_URL`                      |         | No       | LLM API URL for triage decisions                                                    |
| `TRIAGE_LLM_API_KEY`                  |         | No       | API key for the triage LLM                                                          |
| `TRIAGE_LLM_MODEL`                    |         | No       | LLM model for triage                                                                |
| `TRIAGE_MAX_CONCURRENT`               | `3`     | No       | Max concurrent triage operations                                                    |
| `TRIAGE_TIMEOUT`                      | `2m`    | No       | Triage operation timeout                                                            |
| `MAX_CONCURRENT_TRIAGE`               | `5`     | No       | Max concurrent triage workers                                                       |
| `TRIAGE_CONFIDENCE_THRESHOLD`         | `0.7`   | No       | Min confidence for auto-decisions; below this, decisions downgrade to `enrich_only` |
| `TRIAGE_AUTO_RESOLVE_ENABLED`         | `false` | No       | Allow `auto_resolve` decisions (otherwise downgrades to `enrich_only`)              |
| `TRIAGE_SUPPRESS_ENABLED`             | `false` | No       | Allow `suppress` decisions (otherwise downgrades to `enrich_only`)                  |
| `TRIAGE_CONTEXT_EPISODIC_LIMIT`       | `3`     | No       | Max episodic context entries                                                        |
| `TRIAGE_CONTEXT_NOTES_LIMIT`          | `3`     | No       | Max knowledge notes for context                                                     |
| `TRIAGE_CONTEXT_MEMORIES_LIMIT`       | `5`     | No       | Max agent memories for context                                                      |
| `TRIAGE_AUTO_PROMOTE_CONFIRMED_COUNT` | `3`     | No       | Count of confirmed triages before auto-promotion                                    |

## Google OAuth

Enables "Sign in with Google" on the login page.

| Variable                    | Default | Required | Description                                                           |
| --------------------------- | ------- | -------- | --------------------------------------------------------------------- |
| `GOOGLE_CLIENT_ID`          |         | No       | Google OAuth client ID                                                |
| `GOOGLE_CLIENT_SECRET`      |         | No       | Google OAuth client secret                                            |
| `GOOGLE_OAUTH_REDIRECT_URL` |         | No       | Override callback URL (auto-detected from request headers if not set) |
| `GOOGLE_OAUTH_ENABLED`      | `true`  | No       | Toggle Google Sign-In                                                 |

## Google Meet (War Rooms)

Auto-creates a Google Meet space per incident for war-room coordination. Requires a service-account JSON with the Meet API enabled and domain-wide delegation for scope `https://www.googleapis.com/auth/meet.space.admin`.

| Variable                       | Default | Required | Description                                           |
| ------------------------------ | ------- | -------- | ----------------------------------------------------- |
| `GOOGLE_MEET_ENABLED`          | `false` | No       | Enable Google Meet integration                        |
| `GOOGLE_MEET_CREDENTIALS_PATH` |         | No       | Path to the service-account JSON                      |
| `GOOGLE_MEET_AUTO_CREATE`      | `true`  | No       | Auto-create a Meet space when an incident goes active |

## Data Retention

| Variable              | Default | Required | Description                                                |
| --------------------- | ------- | -------- | ---------------------------------------------------------- |
| `DATA_RETENTION_DAYS` | `90`    | No       | Days to retain resolved alerts. Set to `0` to keep forever |

## Observability (OpenTelemetry)

Tracing is env-gated and off by default. When disabled, a noop tracer provider is installed and tracing has zero runtime cost. Tracing turns on when `ALGA_OTEL_ENABLED=true` or an OTLP endpoint is configured.

| Variable                      | Default                      | Required | Description                                                                                     |
| ----------------------------- | ---------------------------- | -------- | ----------------------------------------------------------------------------------------------- |
| `ALGA_OTEL_ENABLED`           | `false`                      | No       | Master switch for OpenTelemetry tracing                                                         |
| `OTEL_EXPORTER_OTLP_ENDPOINT` |                              | No       | OTLP/HTTP collector endpoint. The standard `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is also honored |
| `ALGA_OTEL_SAMPLE_RATIO`      | `1.0` (when tracing enabled) | No       | Fraction (0.0–1.0) of new traces sampled via a ParentBased(TraceIDRatioBased) sampler           |

## Frontend

The frontend is a Vite app; its variables are prefixed with `VITE_` and read at build time.

| Variable            | Default | Required | Description                                                                   |
| ------------------- | ------- | -------- | ----------------------------------------------------------------------------- |
| `VITE_API_BASE_URL` |         | No       | API base URL override for the frontend. Leave empty to use the Vite dev proxy |

## Features With No Environment Variables

These features are configured entirely through the API/UI, not environment variables:

- **Playbooks** — managed via `/api/v1/playbooks`
- **Heartbeats** — managed via `/api/v1/heartbeats`
- **Status Pages** — managed via `/api/v1/status-pages`
- **Credential Providers & Shared Secrets** — managed via `/api/v1/credential-providers` and `/api/v1/shared-secrets`
- **Incident Summaries** — `incident_summary_enabled`, `incident_summary_interval`, and per-severity `incident_summary_intervals` are managed via the System Configuration API
- **On-Call Reminders** — on-call shift handoff detection runs on the scheduler; reminder behavior is not controlled by environment variables

## System Configuration API

Runtime settings can also be managed via the API at `PUT /api/v1/system/config`. For supported settings this overrides environment variables. See [System Configuration API](/configuration/system-config).
