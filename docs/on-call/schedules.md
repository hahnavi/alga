---
title: On-Call Schedules
description: Multi-layer on-call schedules with rotations, overrides, follow-the-sun coverage, reminders, handoffs, and pager-load metrics.
---

# On-Call Schedules

Alga supports multi-layer on-call schedules with follow-the-sun restrictions and overrides.

## Schedule Structure

A schedule's display name is **derived dynamically from its owning team** — no `name` field is stored on the schedule itself. There is exactly one schedule per team, auto-provisioned when the team is created.

Each schedule has one or more **layers** that define rotating on-call coverage. A layer carries:

| Field | Description |
|-------|-------------|
| `name` | Display name for the layer (e.g., "Primary", "Secondary") |
| `rotation_type` | `weekly`, `daily`, or `custom` |
| `rotation_interval` | How many units of `rotation_type` per shift (e.g., `2` for bi-weekly) |
| `start_date` | When the rotation begins (RFC3339) |
| `end_date` | Optional end; the layer stops resolving after this |
| `timezone` | IANA timezone the layer's windows are interpreted in (default `UTC`) |
| `start_time` / `end_time` | Daily active window as `HH:MM` in the layer's timezone; empty `end_time` means active all day |
| `days_of_week` | Active days (empty means every day) |
| `priority` | Higher-priority layers win when multiple layers are active |
| `user_ids` | Ordered list of users in the rotation |

When more than one layer is active at a given moment, the layer with the highest `priority` determines who is on call. Users added to a layer must be members of the schedule's team and must have a phone number on file.

## Overrides

Temporary schedule changes can be created via **overrides**. An override records `user_id`, `start_at`, `end_at`, and `created_by`:

- Covers a specific time range
- Takes **absolute precedence** over every layer — while an override is active, the override's user is on call regardless of layer priority or rotation
- Useful for PTO, sick days, or manual coverage swaps

## Viewing On-Call Status

### Who is On-Call Now?
- **Global**: `GET /api/v1/on-call/who-is-on-call` — All schedules' current on-call
- **Per Schedule**: `GET /api/v1/on-call/schedules/{id}/current` — Specific schedule
- **My Shifts**: `GET /api/v1/on-call/me` — Current/pending shifts for the logged-in user

### Timeline
`GET /api/v1/on-call/schedules/{id}/timeline` — Shows next N rotations

## Handoffs

On-call **handoffs** formalize the transfer of pager responsibility between responders. Each handoff record includes:

- **Outgoing / Incoming user** — the responder ending their shift and the one taking over
- **Handoff time** (`handoff_at`) — when the transfer occurs
- **Status** — `pending` until the incoming responder acknowledges, then `acknowledged`
- **Outgoing notes** — context from the outgoing responder (open issues, items to watch); editable only by the outgoing user
- **Incoming notes** — notes from the incoming responder; editable only by the incoming user
- **Incident summary** — a snapshot of active incidents at handoff time

Handoffs ensure continuity of on-call coverage and reduce the risk of dropped context during shift transitions. Only the incoming user can acknowledge a handoff, and pending handoffs are surfaced prominently so nothing falls through the cracks.

## Pager Load Metrics

Track on-call burden per shift to identify overloaded responders and optimize rotation design. `GET /api/v1/on-call/metrics` requires `schedule_id`, `start_date`, and `end_date` (RFC3339) query parameters and returns per-shift statistics:

- **Alerts received / acknowledged / resolved / missed** — alert volume and outcomes during each shift
- **Average ack time** (`avg_ack_time_seconds`) — mean time to acknowledge during the shift
- **Summary** — `total_shifts`, `avg_ack_rate`, and `avg_ack_time_seconds` across the range

Pass `group_by=user` to aggregate shifts per responder instead of per shift.

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
| `GET` | `/api/v1/on-call/my-on-call` | Session | `oncall:read` | Alias for `/on-call/me` |
| `GET` | `/api/v1/on-call/schedules/{id}/current` | Session | `oncall:read` | Current on-call for schedule |
| `GET` | `/api/v1/on-call/schedules/{id}/timeline` | Session | `oncall:read` | Next N rotations |
| `GET` | `/api/v1/on-call/schedules/{id}/ical` | Session | `oncall:read` | Export shifts as iCalendar (.ics) |

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
| `POST` | `/api/v1/on-call/handoffs/{id}/notes` | Session | `oncall:write` | Save notes (outgoing_notes or incoming_notes) |
| `POST` | `/api/v1/on-call/handoffs/{id}/acknowledge` | Session | `oncall:write` | Acknowledge |

### Pager Load Metrics
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/metrics` | Session | `oncall:read` | Pager load metrics |

## Agent API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/agent/on-call/current` | Bearer | Who is on call |
