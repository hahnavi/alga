---
title: Heartbeats
description: Dead-man's-switch monitoring — configure periodic ping URLs so Alga alerts you when a cron job, scheduled task, or service stops reporting.
---

# Heartbeats

Heartbeats are dead-man's-switch monitors. A periodic job pings a unique Alga URL; if a ping stops arriving within the configured interval plus grace period, Alga creates an alert so you can respond to the silent failure (e.g. a cron job that stopped running, a service that stopped reporting).

## How It Works

1. Create a heartbeat with an `interval_seconds` and optional `grace_seconds`.
2. Configure your job/service to send a `GET` or `HEAD` to the heartbeat's ping URL on every run.
3. Each ping extends the heartbeat's expiry (`now + interval + grace`) and keeps its status `healthy`.
4. If no ping arrives before expiry, the **heartbeat sweep worker** (ticks every 30s) marks the heartbeat `expired` and ingests a firing alert with fingerprint `heartbeat:{id}`.
5. The next ping resolves the alert automatically.

## Ping Endpoint

The ping endpoint is **public** (no auth) — the token in the path **is** the capability. It is rate-limited.

```
GET  /api/v1/heartbeats/ping/{token}
HEAD /api/v1/heartbeats/ping/{token}
```

Example cron entry:

```sh
*/5 * * * * curl -fsS https://alga.example.com/api/v1/heartbeats/ping/alga_hb_... >/dev/null
```

::: tip
The plaintext ping token is shown **once** on create/regenerate. Store it securely — Alga only persists an HMAC hash.
:::

## Configuration

Heartbeats have no environment variables; they are managed entirely through the API/UI.

| Field              | Description                                                                 |
| ------------------ | --------------------------------------------------------------------------- |
| `name`             | Human-readable identifier                                                   |
| `description`      | Optional context                                                            |
| `interval_seconds` | Expected time between pings (positive)                                      |
| `grace_seconds`    | Extra tolerance before breach (default 60)                                  |
| `severity`         | Alert severity if breached: `critical`, `high`, `warning` (default), `info` |
| `labels`           | Label map attached to the generated alert (enables routing rules)           |
| `owner_team_id`    | Owning team                                                                 |
| `enabled`          | Toggle without deleting                                                     |

The generated alert uses labels `{ alertname: "HeartbeatExpired", severity: <severity>, heartbeat: <name>, heartbeat_id: <id> }` (merged with the heartbeat's own labels), so it flows through your normal [routing](/core-features/routing), [escalation](/on-call/escalation-policies), and [notification](/core-features/notifications) setup.

## API

| Method       | Path                                       | Permission          | Description                                                     |
| ------------ | ------------------------------------------ | ------------------- | --------------------------------------------------------------- |
| `GET`        | `/api/v1/heartbeats`                       | `heartbeats:read`   | List heartbeats (query: enabled, status, search, owner_team_id) |
| `POST`       | `/api/v1/heartbeats`                       | `heartbeats:write`  | Create heartbeat                                                |
| `GET`        | `/api/v1/heartbeats/{id}`                  | `heartbeats:read`   | Get heartbeat                                                   |
| `PUT`        | `/api/v1/heartbeats/{id}`                  | `heartbeats:write`  | Update heartbeat                                                |
| `DELETE`     | `/api/v1/heartbeats/{id}`                  | `heartbeats:delete` | Delete heartbeat                                                |
| `POST`       | `/api/v1/heartbeats/{id}/regenerate-token` | `heartbeats:write`  | Regenerate ping token                                           |
| `GET`/`HEAD` | `/api/v1/heartbeats/ping/{token}`          | None                | Record a ping                                                   |
