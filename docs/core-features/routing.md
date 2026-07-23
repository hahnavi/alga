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

| Operator | Description | Example |
|----------|-------------|---------|
| `exact` | Exact string match | `namespace: production` |
| `contains` | Contains substring | `message: error` |
| `prefix` | Starts with | `pod: api-` |
| `suffix` | Ends with | `cluster: -prod` |
| `wildcard` | Glob pattern (`*` matches any) | `host: web-*.example.com` |
| `regex` | Regular expression | `pod: api-server-\d+` |
| `exists` | Label key exists | `namespace` exists |
| `not_exists` | Label key does not exist | `namespace` does not exist |

## Match Mode

When a rule has multiple conditions, the `match_mode` field controls how they are combined:

| Match Mode | Behavior |
|------------|----------|
| `"all"` (default) | ALL conditions must match (AND logic) |
| `"any"` | At least one condition must match (OR logic) |

## Condition Source

By default, conditions match against alert labels. Use the `source` field to match against other alert attributes:

| Source | Description | Matchable Fields |
|--------|-------------|------------------|
| `labels` (default) | Alert labels | Any label key |
| `annotations` | Alert annotations | Any annotation key |
| `alert` | Alert fields | `status`, `fingerprint`, `generator_url`, `silence_url`, `dashboard_url`, `panel_url`, `alertname` |

## Silenced Rules

Rules with `silenced: true` suppress matching alerts entirely — no routing, no notifications, no investigation triggers. This is useful for permanently ignoring known noise without deleting the rule.

## Correlation Rules

By default, Alga correlates alerts using a combination of deployment name and alertname within the configured `CORRELATION_WINDOW`. Correlation rules let you customize the correlation key per alertname so that related alerts are grouped together for investigation.

For example, you can correlate all alerts from the same namespace and service regardless of alertname, or keep strict per-alertname correlation for noisy alerts.

Correlation rules are configured alongside routing rules and apply during the alert correlation phase, before investigation dispatch.

## Suppression Rules

Suppression rules allow you to suppress alerts based on label matchers before they enter the routing and notification pipeline. Suppressed alerts are still ingested and stored, but they do not trigger:

- Routing to any destination
- Notification dispatch
- Investigation triggers
- Alert correlation

This is useful for temporarily silencing known issues during maintenance or for permanently dropping noise from specific sources.

Suppression rules differ from silenced routing rules in that they are evaluated earlier in the pipeline and can be managed separately from routing configuration.

## Multiple Conditions

When a rule has multiple conditions, ALL must match (AND logic).

## Default Destinations

When no rule matches, alerts are sent to the configured default destinations:
- `SLACK_DEFAULT_CHANNEL` for Slack
- `MATTERMOST_DEFAULT_CHANNEL` for Mattermost

## Maintenance Windows

Create maintenance windows to suppress alerts during planned maintenance:

1. Go to **Maintenance** in the sidebar
2. Create a window with:
   - Name (e.g., "Database Migration")
   - Start and end time
   - Label matchers (e.g., `namespace: database`)
3. Matching alerts are still ingested but **suppressed** — no routing, notifications, or investigation triggers

## Escalation

Alga supports severity-based escalation:

| Severity | Behavior |
|----------|----------|
| `info` | Notification only |
| `warning` | Notification + optional investigation |
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
