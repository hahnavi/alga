---
title: Alerts
description: Alert lifecycle, webhook ingestion format, Grafana setup, fingerprint-based deduplication, search, threading, correlation, and retention.
---

# Alerts

## Alert Lifecycle

Alga follows the Opsgenie deduplication model:

1. **Firing** → Alert is active and needs attention
2. **Acknowledged** → Someone is looking into it
3. **Resolved** → Alert is no longer active

Key concepts:

- **Fingerprint** is a dedup key, not a unique identifier. Multiple resolved alerts can share the same fingerprint.
- **Alert Number** is the true unique identifier for each alert, assigned sequentially via the counters table.
- Resolved alerts are never reopened _automatically_ by the system, but they **can** be manually reopened via the API (`POST /api/v1/alerts/{alert_number}/reopen`) or the UI. Reopening resets the alert from `resolved` back to `firing` and can revive a terminal investigation (except `promoted` — a promoted investigation is irreversible). A new firing alert with the same fingerprint creates a fresh alert record.

### Alert Fields

| Field                 | Description                                                |
| --------------------- | ---------------------------------------------------------- |
| `alert_number`        | Sequential unique identifier (from counters table)         |
| `fingerprint`         | Dedup key for correlating related alerts                   |
| `status`              | `firing` or `resolved`                                     |
| `acknowledged`        | Whether the alert has been acknowledged                    |
| `silenced`            | Whether the alert is silenced                              |
| `labels`              | Key-value label map (e.g., alertname, severity, namespace) |
| `annotations`         | Key-value annotation map (e.g., summary, description)      |
| `values`              | Metric values associated with the alert                    |
| `starts_at`           | When the alert started firing                              |
| `ends_at`             | When the alert was resolved                                |
| `generator_url`       | Link back to the source (e.g., Grafana)                    |
| `triage_result_id`    | Linked triage result (if triaged)                          |
| `enrichment`          | Enriched metadata from triage or agents                    |
| `triage_category`     | Category assigned by triage                                |
| `severity_classified` | Severity determined by triage                              |
| `deleted_at`          | Soft-delete tombstone timestamp (null if active)           |

### Soft Delete

Alerts use a **soft delete** model: deleting an alert sets the `deleted_at` timestamp rather than removing the row. Soft-deleted alerts are excluded from normal queries but remain in the database for audit and retention purposes.

### Partial Unique Index

A partial unique index enforces **one open alert per fingerprint**: only one alert with a given fingerprint may exist where `status != resolved AND deleted_at IS NULL`. This prevents duplicate firing alerts for the same source while allowing historical resolved records to accumulate.

### Resolve Cascade

Resolving an incident automatically resolves all linked firing alerts associated with that incident.

## Ingestion

Send alerts to Alga via webhook:

```
POST /webhooks/alerts
Authorization: Bearer YOUR_WEBHOOK_TOKEN
Content-Type: application/json
```

### Payload Format

Alga accepts Grafana-compatible alert payloads:

```json
{
  "alerts": [
    {
      "status": "firing",
      "labels": {
        "alertname": "HighCPU",
        "severity": "critical",
        "namespace": "production",
        "pod": "api-server-7d4f8b"
      },
      "annotations": {
        "summary": "CPU usage above 90%",
        "description": "Pod api-server-7d4f8b has been at 95% CPU for 5 minutes"
      },
      "startsAt": "2026-05-05T10:00:00Z",
      "generatorURL": "http://grafana:3000/alerting/list"
    }
  ],
  "commonLabels": {
    "alertname": "HighCPU"
  },
  "commonAnnotations": {},
  "externalURL": "http://grafana:3000",
  "receiver": "alga-webhook",
  "status": "firing",
  "groupKey": "{}/{}/{}:{alertname=\"HighCPU\"}"
}
```

### Authentication

Include the webhook token as either:

- `Authorization: Bearer alga_...` header
- `?token=alga_...` query parameter

## Grafana Setup

1. In Alga, go to **Settings → Webhook Tokens** and create a token
2. In Grafana, navigate to **Alerting → Contact points → Add contact point**
3. Set type to **Webhook**
4. Set URL to: `http://your-alga-host:8080/webhooks/alerts?token=alga_YOUR_TOKEN`
5. Optional: Set HTTP method to `POST`
6. Save and test the contact point

## Manual Alert Creation

Create alerts directly from the UI or API with extended fields:

```json
POST /api/v1/alerts
{
  "summary": "Manual alert",
  "source": "manual",
  "labels": {
    "alertname": "DiskFull",
    "severity": "warning",
    "namespace": "staging"
  },
  "annotations": {
    "description": "Disk usage at 85% on staging-db-01"
  }
}
```

The `source` field defaults to `grafana` for webhook alerts. Manual alerts use `manual` or a custom source string. Labels and annotations follow the same format as Grafana payloads and are used by routing rules and correlation.

## Alert Actions

| Action      | Endpoint                                         | Description                                                                 |
| ----------- | ------------------------------------------------ | --------------------------------------------------------------------------- |
| List        | `GET /api/v1/alerts`                             | List with filters (status, severity, channel, provider, search, date range) |
| Get         | `GET /api/v1/alerts/{alert_number}`              | Get alert and active investigation                                          |
| Create      | `POST /api/v1/alerts`                            | Create a manual alert                                                       |
| Acknowledge | `POST /api/v1/alerts/{alert_number}/acknowledge` | Mark as acknowledged                                                        |
| Resolve     | `POST /api/v1/alerts/{alert_number}/resolve`     | Manually resolve                                                            |
| Reopen      | `POST /api/v1/alerts/{alert_number}/reopen`      | Reopen resolved alert                                                       |
| Investigate | `POST /api/v1/alerts/{alert_number}/investigate` | Trigger AI investigation                                                    |
| Delete      | `DELETE /api/v1/alerts/{alert_number}`           | Delete alert (alerts:delete)                                                |
| Related     | `GET /api/v1/alerts/{alert_number}/related`      | Get correlated alerts and linked incident                                   |

Agent endpoints use fingerprint-based routing — see [Agent REST API](/api-reference/#agent-rest-api).

## Alert Events

Each alert tracks a timeline of events (acknowledge, resolve, reopen, investigate, etc.). Each event records:

| Field        | Description                                                                        |
| ------------ | ---------------------------------------------------------------------------------- |
| `type`       | Event type (e.g., `acknowledged`, `resolved`, `reopened`, `investigation_started`) |
| `timestamp`  | When the event occurred                                                            |
| `actor_type` | Who performed the action (`user`, `agent`, `system`)                               |
| `actor_id`   | Identifier of the actor                                                            |
| `source`     | Origin of the event (e.g., `api`, `webhook`, `scheduler`)                          |

## Alert Search

Full-text search across alert summaries, labels, and annotations:

```
GET /api/v1/alerts?search=HighCPU&status=firing&limit=20
```

The `search` query parameter matches against alert summaries, label values, and annotation values. Combine with filters for status, severity, channel, provider, and date ranges (`start_date`, `end_date`).

## Alert Threads

Each alert has a dedicated **owner-thread** for operator discussion. Thread messages support real-time updates via SSE.

| Method | Endpoint                                        | Description                 |
| ------ | ----------------------------------------------- | --------------------------- |
| `GET`  | `/api/v1/alerts/{alert_number}/thread`          | Get thread with messages    |
| `POST` | `/api/v1/alerts/{alert_number}/thread/messages` | Add a message to the thread |
| `POST` | `/api/v1/alerts/{alert_number}/thread/typing`   | Typing indicator            |

New messages are pushed to connected clients over the SSE stream (`GET /api/v1/events`), so the thread UI updates in real time without polling.

## Related Alerts

View correlated alerts that were grouped into the same investigation:

```
GET /api/v1/alerts/{alert_number}/related
```

Returns related alerts sharing the same correlation key and the linked incident (if any). Useful for understanding blast radius during an active incident.

## Alert Correlation

When `CORRELATION_WINDOW` is set (e.g., `5m`), alerts that share the same correlation key (deployment name + alertname) within the window are grouped into a single investigation.

The correlator:

1. Buffers incoming alerts by correlation key
2. Waits for the window to expire
3. Creates a single investigation covering all correlated alerts
4. Honors `CORRELATION_COOLDOWN_TTL` (default `30m`) to prevent duplicate investigations on the same key

| Variable                   | Default        | Description                                                           |
| -------------------------- | -------------- | --------------------------------------------------------------------- |
| `CORRELATION_WINDOW`       | `0` (disabled) | Time window for grouping related alerts                               |
| `CORRELATION_COOLDOWN_TTL` | `30m`          | Cooldown before a new investigation opens on the same correlation key |

## Delivery Targets

Alerts track where they were delivered — Mattermost post IDs, Slack message timestamps, and other delivery references. This enables bidirectional sync: replies in Mattermost or Slack threads are reflected back into Alga, and Alga updates are pushed to the original channel post.

Each delivery target stores:

| Field      | Description                                     |
| ---------- | ----------------------------------------------- |
| `provider` | Delivery provider (e.g., `mattermost`, `slack`) |
| `channel`  | Channel or conversation ID                      |
| `post_id`  | External post/message ID in the provider        |

## Maintenance Window Suppression

Alerts that fire during an active maintenance window are suppressed. The alert is still stored in the database with its full labels and annotations, but no routing, notification, or investigation is triggered. When the maintenance window expires, subsequent alerts on the same fingerprint resume normal processing.

Configure maintenance windows under **Routes → Maintenance Windows** or via the API:

| Method   | Endpoint                           | Description               |
| -------- | ---------------------------------- | ------------------------- |
| `GET`    | `/api/v1/maintenance-windows`      | List maintenance windows  |
| `POST`   | `/api/v1/maintenance-windows`      | Create maintenance window |
| `PUT`    | `/api/v1/maintenance-windows/{id}` | Update maintenance window |
| `DELETE` | `/api/v1/maintenance-windows/{id}` | Delete maintenance window |

Each window has a start time, end time, and optional label selectors to scope suppression to specific alerts.

## Stale Alert Sweep

A background sweeper detects firing alerts that have not been investigated within a configurable threshold and marks them as stale candidates for re-investigation.

| Variable                     | Default | Description                                                  |
| ---------------------------- | ------- | ------------------------------------------------------------ |
| `STALE_ALERT_THRESHOLD`      | `15m`   | Alert age before the stale sweep considers it uninvestigated |
| `STALE_ALERT_SWEEP_INTERVAL` | `5m`    | How often the stale sweep runs                               |

## Data Retention

Resolved alerts are automatically pruned based on `DATA_RETENTION_DAYS` (default: 90 days). Set to `0` to keep alerts forever.

### Manual Pruning

```sh
# Dry run: count alerts that would be deleted
./alga data prune --dry-run

# Prune with custom retention
./alga data prune --days 30

# Prune using configured retention
./alga data prune
```

The retention scheduler runs hourly when `DATA_RETENTION_DAYS > 0`.
