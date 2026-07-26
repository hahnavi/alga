---
title: Routing
description: Route alerts to the right channel, team, or agent using first-match rules with label and annotation conditions, suppression windows, and maintenance mode.
---

# Routing

Alga's routing engine evaluates alert labels against configurable rules to determine where alerts should be sent.

## How Routing Works

When an alert arrives, the routing engine evaluates rules in order. The **first matching rule** wins. If no rule matches, the **default destinations** are used.

## Rule Structure

Each rule has:

- **Name** — human-readable identifier
- **Conditions** — one or more label/field matchers
- **Destinations** — Slack channels and/or Mattermost channels to notify

## Condition Operators

| Operator     | Description                    | Example                    |
| ------------ | ------------------------------ | -------------------------- |
| `exact`      | Exact string match             | `namespace: production`    |
| `contains`   | Contains substring             | `message: error`           |
| `prefix`     | Starts with                    | `pod: api-`                |
| `suffix`     | Ends with                      | `cluster: -prod`           |
| `wildcard`   | Glob pattern (`*` matches any) | `host: web-*.example.com`  |
| `regex`      | Regular expression             | `pod: api-server-\d+`      |
| `exists`     | Label key exists               | `namespace` exists         |
| `not_exists` | Label key does not exist       | `namespace` does not exist |

## Match Mode

When a rule has multiple conditions, the `match_mode` field controls how they are combined:

| Match Mode        | Behavior                                     |
| ----------------- | -------------------------------------------- |
| `"all"` (default) | ALL conditions must match (AND logic)        |
| `"any"`           | At least one condition must match (OR logic) |

## Condition Source

By default, conditions match against alert labels. Use the `source` field to match against other alert attributes:

| Source             | Description       | Matchable Fields                                                                                   |
| ------------------ | ----------------- | -------------------------------------------------------------------------------------------------- |
| `labels` (default) | Alert labels      | Any label key                                                                                      |
| `annotations`      | Alert annotations | Any annotation key                                                                                 |
| `alert`            | Alert fields      | `status`, `fingerprint`, `generator_url`, `silence_url`, `dashboard_url`, `panel_url`, `alertname` |

## Silenced Rules

Rules with `silenced: true` suppress matching alerts entirely — no routing, no notifications, no investigation triggers. This is useful for permanently ignoring known noise without deleting the rule.

## Correlation

Before routing, Alga correlates related alerts into a single investigation. The correlation key is derived deterministically from the alert's labels: the first workload-identity label found (`deployment`, `statefulset`, `daemonset`, or `job`) combined with `namespace` and `alertname`. Alerts that share a key within the configured `CORRELATION_WINDOW` are grouped into the same investigation rather than spawning duplicate work.

Correlation is automatic and not configured per-rule. To stop alerts from generating investigations at all, use one of the suppression mechanisms below.

## Suppressing Alerts

There are two user-facing ways to suppress alerts:

- **Silenced routing rules** — a rule with `silenced: true` drops matching alerts before routing (see above).
- **Maintenance windows** — time-boxed, label-matched suppression for planned maintenance (see below).

In both cases the alert is still ingested and stored, but it does not trigger routing, notification dispatch, or investigation creation.

## Multiple Conditions

When a rule has multiple conditions, the `match_mode` field controls how they combine — `"all"` (default) requires every condition to match, while `"any"` requires at least one. See [Match Mode](#match-mode) above.

## Default Destinations

When no rule matches, alerts are sent to the configured default destinations:

- `SLACK_DEFAULT_CHANNEL` for Slack
- `MATTERMOST_DEFAULT_CHANNEL` for Mattermost

## Maintenance Windows

Create maintenance windows to suppress alerts during planned maintenance (see [Maintenance Windows](/core-features/maintenance-windows) for the full reference):

1. Go to **Maintenance** in the sidebar
2. Create a window with:
   - Name (e.g., "Database Migration")
   - Start and end time
   - Label matchers (e.g., `namespace: database`)
3. Matching alerts are still ingested but **suppressed** — no routing, notifications, or investigation triggers

## Escalation

Alga supports severity-based escalation:

| Severity   | Behavior                                           |
| ---------- | -------------------------------------------------- |
| `info`     | Notification only                                  |
| `warning`  | Notification + optional investigation              |
| `critical` | Notification + investigation + optional voice call |

Configure critical escalation with:

- `CRITICAL_SEVERITY_LABELS` — comma-separated labels that trigger critical escalation
- `INVESTIGATION_CHANNEL` — Mattermost channel for investigation threads (Slack uses channel routing via rules)
- Voice call configuration. Voice escalation supports either **Twilio** or **Telnyx**, selected by the `VOICE_PROVIDER` env var (`twilio` or `telnyx`). See the [Twilio integration](/integrations/twilio) docs.

## API Endpoints

```
GET  /api/v1/routes              # Get routing rules and defaults
PUT  /api/v1/routes              # Save routing rules
GET  /api/v1/channels            # List Mattermost channels
GET  /api/v1/destinations        # List channels (supports ?provider=mattermost|slack)
```
