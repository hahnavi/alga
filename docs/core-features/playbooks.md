---
title: Playbooks
description: Structured response procedures matched to investigations via label selectors, injected into agent dispatch prompts for guided remediation.
---

# Playbooks

Playbooks provide structured, step-by-step response procedures that agents and operators can follow during incidents. They can be automatically matched to investigations based on alert labels.

## Overview

Playbooks define repeatable response procedures for common operational scenarios:

- **Procedure** playbooks document standard operating procedures (SOPs)
- **Mitigation** playbooks guide incident response and remediation
- **Label selectors** automatically match playbooks to alert contexts
- **Ordered steps** with titles, descriptions, durations, and optional commands
- Automatic enrichment into investigation prompts when labels match

## Playbook Fields

| Field | Type | Description |
|-------|------|-------------|
| `title` | `string` | Playbook name |
| `kind` | `string` | `procedure` or `mitigation` |
| `summary` | `string` | Brief description of the playbook's purpose |
| `service_id` | `UUID` | Optional linked service for scoping |
| `label_selectors` | `array` | Flat key→value maps for matching alert labels (exact equality; prefix value with `~` for regex) |
| `tags` | `array` | Categorization tags for filtering |
| `steps` | `array` | Ordered list of execution steps |

## Steps

Each step within a playbook contains:

| Field | Type | Description |
|-------|------|-------------|
| `title` | `string` | Step name |
| `description` | `string` | Detailed instructions |
| `expected_duration_minutes` | `int` | Estimated time to complete |
| `command` | `string` | Optional shell command to execute |

## Creating Playbooks

Create a playbook with label selectors and steps:

```sh
curl -X POST http://localhost:8080/api/v1/playbooks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Database Connection Pool Exhaustion",
    "kind": "mitigation",
    "summary": "Respond to database connection pool exhaustion alerts",
    "label_selectors": [
      {"alertname": "DBConnectionPoolExhausted"}
    ],
    "tags": ["database", "connectivity"],
    "steps": [
      {
        "title": "Check active connections",
        "description": "Query pg_stat_activity for active connection count and wait events",
        "expected_duration_minutes": 5,
        "command": "psql -c \"SELECT count(*) FROM pg_stat_activity\""
      },
      {
        "title": "Identify long-running queries",
        "description": "Find queries running longer than 60 seconds",
        "expected_duration_minutes": 5
      },
      {
        "title": "Terminate blocked queries",
        "description": "Terminate queries that are blocking connection pool slots",
        "expected_duration_minutes": 3
      }
    ]
  }'
```

## API Endpoints

### Playbook Management

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/playbooks` | `playbooks:read` | List playbooks (query: `kind`, `service_id`, `tag`, `search`, `limit`, `skip`) |
| `POST` | `/api/v1/playbooks` | `playbooks:write` | Create playbook |
| `GET` | `/api/v1/playbooks/{id}` | `playbooks:read` | Get playbook with steps |
| `PATCH` | `/api/v1/playbooks/{id}` | `playbooks:write` | Update playbook |
| `DELETE` | `/api/v1/playbooks/{id}` | `playbooks:write` | Delete playbook |

### Step Management

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `POST` | `/api/v1/playbooks/{id}/steps` | `playbooks:write` | Add step |
| `PATCH` | `/api/v1/playbooks/{id}/steps/{stepId}` | `playbooks:write` | Update step |
| `DELETE` | `/api/v1/playbooks/{id}/steps/{stepId}` | `playbooks:write` | Delete step |
| `PUT` | `/api/v1/playbooks/{id}/steps/reorder` | `playbooks:write` | Reorder steps |

## Automatic Matching

The investigation scheduler automatically matches playbooks to investigations:

1. When an investigation is created, the scheduler evaluates label selectors against the alert's labels
2. Matched playbooks are included in the investigation prompt sent to the agent
3. Steps are displayed in the incident detail sidebar under **Mitigation Playbooks**
4. Agents can reference playbook steps during investigation and mark them as completed

Label selectors are a flat map of label key to value, matched with **exact equality**. There is no `operator` field. To match with a regular expression instead, prefix the value with `~`.

```json
// Exact match: labels.alertname == "DBConnectionPoolExhausted"
{"alertname": "DBConnectionPoolExhausted"}

// Regex match: labels.namespace matches the pattern
{"namespace": "~prod-.*"}
```

A selector matches when **all** of its key/value pairs match the alert's labels. Multiple selectors in the `label_selectors` array are OR'd — a playbook matches if **any** selector matches.

## See Also

- [AI Investigation](/core-features/investigation) — investigation lifecycle and agent communication
- [Knowledge Base](/core-features/knowledge-base) — operator-curated notes for agents
- [Incident Lifecycle](/incident-management/lifecycle) — incident state machine and transitions
