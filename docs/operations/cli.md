---
title: CLI Reference
description: Full alga CLI command reference — server, webhook tokens, alerts, database, data retention, demo data, triage, and users.
---

# CLI Reference

The `alga` binary serves double duty: it is both the HTTP server and the command-line management tool. All CLI commands are subcommands of the same binary you use to run the backend.

## Global Flags

All commands load configuration from environment variables and `.env` files (see [Environment Variables](/configuration/environment-variables)).

## Server Commands

### `alga`

Start the Alga HTTP server (default command when no subcommand is given).

```sh
# From source
go run .            # inside apps/backend/

# From built binary
./alga
```

The server listens on the port specified by `PORT` (default: `8080`). On startup it:

1. Validates configuration
2. Runs auto-migration if `POSTGRES_AUTO_MIGRATE=true`
3. Connects to Valkey and RabbitMQ (if configured)
4. Redirects to the first-run setup wizard when no users exist (the operator creates the initial admin account via the UI)
5. Starts background workers and the scheduler
6. Serves the REST API, webhooks, and SSE streams

See [Deployment](/operations/deployment) for production setup and [Architecture](/operations/architecture) for component details.

## Version

### `alga version`

Print the build version of the `alga` binary.

```sh
alga version
```

Output:

```
v1.4.0
```

The version is injected at build time via `-ldflags`; development builds print `dev`.

## Webhook Token Management

Webhook tokens authenticate external alert sources sending to `POST /webhooks/alerts`.

### `alga webhook-token generate <name>`

Generate a new webhook token. The full token is shown **once** — save it securely.

```sh
alga webhook-token generate grafana-prod
```

Output:

```
Webhook token created successfully:
  ID:    550e8400-e29b-41d4-a716-446655440000
  Name:  grafana-prod
  Token: alga_... (save this - it won't be shown again)

Use this token in the Authorization header:
  Authorization: Bearer <token>
```

### `alga webhook-token list`

List all active webhook tokens.

```sh
alga webhook-token list
```

Output:

```
ID                                    NAME             CREATED
550e8400-e29b-41d4-a716-446655440000  grafana-prod     2026-01-15T10:30:00Z
6ba7b810-9dad-11d1-80b4-00c04fd430c8  prometheus-staging 2026-02-01T14:00:00Z
```

### `alga webhook-token revoke <id>`

Revoke a webhook token by its UUID. Revoked tokens immediately stop accepting webhooks.

```sh
alga webhook-token revoke 550e8400-e29b-41d4-a716-446655440000
```

## Alert Operations

### `alga alerts query`

Query alerts with flag-based filters. Results are returned as JSON.

#### Flags

| Flag           | Default       | Description                                 |
| -------------- | ------------- | ------------------------------------------- |
| `--status`     |               | Filter by status (`firing`, `resolved`)     |
| `--channel`    |               | Filter by notification channel              |
| `--provider`   |               | Filter by provider                          |
| `--severity`   |               | Filter by severity                          |
| `--search`     |               | Search in alertname and labels              |
| `--start_date` |               | Start date (RFC3339)                        |
| `--end_date`   |               | End date (RFC3339)                          |
| `--limit`      | `20`          | Max results to return                       |
| `--skip`       | `0`           | Skip results (pagination offset)            |
| `--sort`       | `-updated_at` | Sort field (prefix with `-` for descending) |

#### Examples

```sh
# All firing alerts
alga alerts query --status firing

# Firing alerts sorted by most recently updated, limited to 10
alga alerts query --status firing --limit 10 --sort -updated_at

# Search for alerts containing "HighCPU"
alga alerts query --search HighCPU

# Critical alerts in production
alga alerts query --severity critical --channel backend

# Paginate with skip
alga alerts query --status firing --limit 10 --skip 20
```

## Database Management

### `alga db migrate`

Run database migrations manually. This applies any pending Ent schema changes to PostgreSQL.

```sh
alga db migrate
```

For auto-migration on startup, set `POSTGRES_AUTO_MIGRATE=true`. See [Migration Guide](/operations/migration) for details.

## Data Management

### `alga data prune`

Delete old resolved alerts according to the configured retention period.

```sh
# Prune using DATA_RETENTION_DAYS from config
alga data prune

# Override retention window
alga data prune --days 30

# Preview what would be deleted
alga data prune --dry-run
```

#### Flags

| Flag        | Default                          | Description                                 |
| ----------- | -------------------------------- | ------------------------------------------- |
| `--dry-run` | `false`                          | Count matching alerts without deleting them |
| `--days`    | `0` (uses `DATA_RETENTION_DAYS`) | Override the retention period in days       |

When `--days` is not set and `DATA_RETENTION_DAYS` is `0` (or unset), the command errors — you must specify a retention period via one of the two options.

### `alga data cleanup-deleted`

Hard-delete stale children of soft-deleted alerts and incidents. This is a one-time backfill that removes investigation artifacts (investigations, events, updates, threads, memories) tied to alerts or incidents that were soft-deleted before cascade-on-delete behavior existed. Idempotent and safe to re-run.

```sh
alga data cleanup-deleted
```

Output:

```
Expunged children of 3 soft-deleted alert(s) and 1 soft-deleted incident(s).
```

## Demo & Seeding

### `alga seed`

Seed sample data for demo and evaluation purposes. Creates:

- **5 sample alerts** — covering critical, warning, and info severities across multiple namespaces (production, staging, monitoring). Includes firing, acknowledged, and resolved states.
- **3 knowledge notes** — runbook-style notes for CPU troubleshooting, network troubleshooting, and a deployment checklist.

```sh
alga seed
```

This command is idempotent for the seed data (repeated runs create new records).

## Triage Operations

### `alga triage stats`

Display triage accuracy and volume statistics.

```sh
alga triage stats
```

Output:

```
Triage Stats:
  Total: 42 (confirmed: 35, overridden: 5, pending: 2)
  Accuracy: 87.5%
  By decision: map[incident:20 investigate:15 noise:7]
```

The accuracy percentage is calculated as `confirmed / (confirmed + overridden) * 100`.

## User Management

### `alga user reset-password <email>`

Reset a user's password. Useful for environments without SMTP configured. By default, a random password is generated.

```sh
# Generate a random password
alga user reset-password admin@alga.local

# Set a specific password
alga user reset-password admin@alga.local --password "NewP@ssw0rd!"
```

#### Flags

| Flag         | Default    | Description                                                        |
| ------------ | ---------- | ------------------------------------------------------------------ |
| `--password` | _(random)_ | New password (generates a random 32-character password if omitted) |

The command also clears any account lockout (`failed_login_attempts` reset, `locked_until` cleared).

## See Also

- [Deployment](/operations/deployment) — deployment options and production setup
- [Migration Guide](/operations/migration) — database migration details
- [Environment Variables](/configuration/environment-variables) — full configuration reference
- [API Reference](/api-reference/) — REST API endpoints
