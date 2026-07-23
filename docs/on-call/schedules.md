---
title: On-Call Schedules
description: Multi-layer on-call schedules with rotations, overrides, follow-the-sun coverage, reminders, handoffs, and pager-load metrics.
---

# On-Call Schedules

Alga supports multi-layer on-call schedules with follow-the-sun restrictions and overrides.

## Schedule Structure

A schedule's display name is **derived dynamically from its owning team** — no `name` field is stored on the schedule itself.

Each schedule has **layers** that define rotating on-call coverage:
- **Layer** — Group of users with a rotation schedule
- **Rotation** — Time period for each user's on-call shift
- **Handoff Time** — When the rotation advances to the next person
- **Follow-the-Sun** — Optional restriction to specific time zones

## Overrides

Temporary schedule changes can be created via **overrides**:
- Covers specific time ranges
- Takes precedence over the regular rotation
- Useful for PTO, sick days, or manual coverage swaps

## Viewing On-Call Status

### Who is On-Call Now?
- **Global**: `GET /api/v1/on-call/who-is-on-call` — All schedules' current on-call
- **Per Schedule**: `GET /api/v1/on-call/schedules/{id}/current` — Specific schedule
- **My Shifts**: `GET /api/v1/on-call/me` — Current/pending shifts for the logged-in user

### Timeline
`GET /api/v1/on-call/schedules/{id}/timeline` — Shows next N rotations

## Reminders

Configure `ON_CALL_REMIND_MINUTES` (default: 15) to send reminders before on-call shifts start. Set `ON_CALL_REMIND_ENABLED` to `false` to disable.

## Handoffs

On-call **handoffs** formalize the transfer of pager responsibility between responders. Each handoff includes:

- **From/To** — The outgoing and incoming on-call responder
- **Notes** — Free-text notes from the outgoing responder (context, open issues, items to watch)
- **Acknowledgment** — The incoming responder explicitly acknowledges the handoff, confirming they are aware of and accepting the shift

Handoffs ensure continuity of on-call coverage and reduce the risk of dropped context during shift transitions. Pending handoffs (not yet acknowledged) are surfaced prominently so nothing falls through the cracks.

## Pager Load Metrics

Track on-call burden per shift to identify overloaded responders and optimize rotation design:

- **Incidents handled** — Number of incidents during the shift
- **Alerts received** — Volume of alerts during the shift
- **Average response time** — Mean time to acknowledge during the shift
- **Shift duration** — Total on-call time

Use `GET /api/v1/on-call/metrics` to retrieve shift-level statistics aggregated by schedule and time range.

## API Endpoints

### Schedule Management

> **Note:** Schedules are **auto-provisioned one-per-team**. Creating a team (`POST /api/v1/teams`) automatically creates its on-call schedule. Schedules **cannot be created directly** — `POST /api/v1/on-call/schedules` returns HTTP 405 with the message *"schedules are auto-created from teams and cannot be created directly."* Only `GET` and `PATCH` work on `/api/v1/on-call/schedules/{id}`.

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/schedules` | Session | `oncall:read` | List schedules |
| `GET` | `/api/v1/on-call/schedules/{id}` | Session | `oncall:read` | Get schedule with layers |
| `PATCH` | `/api/v1/on-call/schedules/{id}` | Session | `oncall:write` | Update schedule |
| `DELETE` | `/api/v1/on-call/schedules/{id}` | Session | `oncall:write` | Delete schedule |

### On-Call Lookup
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/who-is-on-call` | Session | `oncall:read` | Global on-call status |
| `GET` | `/api/v1/on-call/me` | Session | `oncall:read` | My current/pending shifts |
| `GET` | `/api/v1/on-call/schedules/{id}/current` | Session | `oncall:read` | Current on-call for schedule |
| `GET` | `/api/v1/on-call/schedules/{id}/timeline` | Session | `oncall:read` | Next N rotations |

### Overrides
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/schedules/{id}/overrides` | Session | `oncall:read` | List overrides |
| `POST` | `/api/v1/on-call/schedules/{id}/overrides` | Session | `oncall:write` | Create override |
| `DELETE` | `/api/v1/on-call/overrides/{id}` | Session | `oncall:write` | Delete override |

### Handoffs
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/handoffs` | Session | `oncall:read` | List handoffs |
| `GET` | `/api/v1/on-call/handoffs/{id}` | Session | `oncall:read` | Get handoff |
| `GET` | `/api/v1/on-call/handoffs/pending` | Session | `oncall:read` | Pending handoffs |
| `PATCH` | `/api/v1/on-call/handoffs/{id}/notes` | Session | `oncall:write` | Save notes |
| `POST` | `/api/v1/on-call/handoffs/{id}/acknowledge` | Session | `oncall:write` | Acknowledge |

### Pager Load Metrics
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/metrics` | Session | `oncall:read` | Pager load metrics |

## Agent API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/agent/on-call/current` | Bearer | Who is on call |
