---
title: Maintenance Windows
description: Schedule time-boxed maintenance windows with label matchers to suppress alert routing, notifications, and investigations during planned work.
---

# Maintenance Windows

Maintenance windows suppress alert processing during planned maintenance. While a window is active, alerts matching its label matchers are still ingested and stored, but they do not trigger routing, notification dispatch, or investigation creation — so planned work doesn't page anyone or spawn noise.

## How They Work

When an alert arrives, Alga checks it against every enabled maintenance window whose time range covers the current moment. If the alert's labels satisfy the window's `label_matchers`, the alert is suppressed for the duration of the window.

A window is active when all of the following hold:

- `enabled` is `true`
- The current time is between `start_time` and `end_time`
- The alert's labels match every entry in `label_matchers`

## Window Fields

| Field | Description |
|-------|-------------|
| `name` | Human-readable name (e.g., "Database Migration") |
| `start_time` | When suppression begins (RFC3339) |
| `end_time` | When suppression ends (RFC3339); must be after `start_time` |
| `label_matchers` | Map of label key/value pairs an alert must match to be suppressed |
| `enabled` | Whether the window is active (default `true`) |
| `created_by` | Email of the user who created the window |

An empty `label_matchers` map matches every alert during the window.

## Creating a Window

1. Go to **Maintenance** in the sidebar
2. Create a window with a name, start and end time, and label matchers (e.g., `namespace: database`)
3. Matching alerts are suppressed until the window ends

You can disable a window early by setting `enabled` to `false`, or delete it.

## Relationship to Routing

Maintenance windows are a suppression mechanism alongside [silenced routing rules](/core-features/routing#silenced-rules). Use silenced rules for permanent, label-based noise dropping, and maintenance windows for temporary, time-boxed suppression around planned changes.

## API Endpoints

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/maintenance-windows` | Session | `routes:read` | List windows (supports `?enabled=true`) |
| `POST` | `/api/v1/maintenance-windows` | Session | `routes:write` | Create window |
| `GET` | `/api/v1/maintenance-windows/{id}` | Session | `routes:read` | Get window |
| `PUT` | `/api/v1/maintenance-windows/{id}` | Session | `routes:write` | Update window |
| `DELETE` | `/api/v1/maintenance-windows/{id}` | Session | `routes:write` | Delete window |

## See Also

- [Routing](/core-features/routing) — alert routing rules and silenced rules
- [Alerts](/core-features/alerts) — alert ingestion and lifecycle
