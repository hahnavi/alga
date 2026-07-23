---
title: ICS Incident Command System
description: Formal Incident Command System roles — Incident Commander, Communications Lead, Responder — with auto-assignment, Google Meet war rooms, and multi-agent coordination.
---

# ICS Incident Command System

Alga implements the Incident Command System (ICS) for structured incident response — a hierarchical command structure with formal role assignments, commander handoffs, and war-room coordination. Every incident gets a clearly defined chain of command with specific responsibilities for each role.

## Why ICS?

ICS is the same framework used by emergency responders worldwide (FEMA, fire services, etc.), adapted for technology incidents. It solves three problems during high-stress incidents:

1. **Who's in charge?** — Every incident has exactly one Incident Commander with final authority
2. **Who does what?** — Roles separate command (decision-making), communications (status updates), and response (hands-on fix)
3. **How do we hand off?** — Formal handoff procedures maintain context across shift changes

## ICS Roles

| Role | Description | Who Can Be Assigned |
|------|-------------|---------------------|
| **Incident Commander (IC)** | Overall authority for the incident response. Makes decisions, coordinates responders, drives the incident to resolution. | Any user or agent. Only one active IC at a time. |
| **Communications Lead** | Handles internal and external communications — publishes status updates, manages stakeholder expectations. | Any user or agent. |
| **Responder** | General tactical response role — investigates, implements fixes, runs diagnostics. | Any user or agent. Multiple responders can be assigned. |

::: warning Three Roles Only
Alga implements three ICS roles: `incident_commander`, `communications_lead`, and `responder`. Additional ICS roles from the FEMA framework (Deputy IC, Technical Lead, SME, Scribe, etc.) are not implemented.
:::

## Assigning ICS Roles

### Via the UI

Open any incident detail page and use the **ICS Role Board** to assign or update roles. Each role card shows the assigned person and allows reassignment with a dropdown.

### Via the API

```bash
curl -X POST http://localhost:8080/api/v1/incidents/{id}/ics/roles \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "role": "incident_commander"
  }'
```

You can assign roles to either human users or AI agents. When an AI agent is the IC, it can dispatch tasks to responder and communicator agents — see [Multi-Agent Coordination](#multi-agent-coordination) below.

## Auto-Assignment

On incident creation, the scheduler automatically assigns the IC role based on:

1. The affected service's linked **on-call schedule** — the current on-call person becomes IC
2. If no schedule is linked, the **escalation policy** attached to the service
3. If neither is configured, the IC role is left unassigned for manual assignment

This means well-configured services get instant IC assignment — no manual step needed.

## IC Handoff

When the current IC goes off shift or needs to transfer command, they initiate a formal handoff:

1. Current IC calls `POST /api/v1/incidents/{id}/ics/handoff` with the incoming IC's user ID and optional notes
2. The incoming IC receives a notification
3. The incoming IC acknowledges via `POST /api/v1/incidents/{id}/ics/handoff/{handoffId}/acknowledge`
4. The IC role transfers automatically and a timeline entry is created

```bash
curl -X POST http://localhost:8080/api/v1/incidents/{id}/ics/handoff \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "incoming_user_id": "660e8400-e29b-41d4-a716-446655440001",
    "notes": "Database failover in progress, waiting for replication catch-up. ETA 15 min."
  }'
```

The handoff notes are critical for continuity — include open issues, ongoing actions, and current context.

## Role Management API

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/ics/roles` | `incidents:read` | List ICS role assignments |
| `POST` | `/api/v1/incidents/{id}/ics/roles` | `incidents:command` | Assign ICS role |
| `PATCH` | `/api/v1/incidents/{id}/ics/roles/{roleId}` | `incidents:command` | Update ICS role |
| `POST` | `/api/v1/incidents/{id}/ics/handoff` | `incidents:command` | Initiate IC handoff |
| `POST` | `/api/v1/incidents/{id}/ics/handoff/{handoffId}/acknowledge` | `incidents:command` | Acknowledge handoff |

## Google Meet War Rooms

When an incident is active, the team needs a place to coordinate in real time. Alga can automatically provision a **Google Meet conference space** per incident for war-room-style voice/video coordination.

### How It Works

When `GOOGLE_MEET_ENABLED=true` and an incident transitions to active status:

1. Alga's ICS worker provisions a Google Meet conference via the Google Calendar API
2. The conference link is attached to the incident
3. Responders see a "Join War Room" button on the incident detail page
4. The conference link is included in incident notifications

### Enabling War Rooms

Set these environment variables:

| Variable | Description |
|----------|-------------|
| `GOOGLE_MEET_ENABLED` | Set to `true` to enable war-room provisioning |
| `GOOGLE_MEET_CREDENTIALS_JSON` | Google service account JSON (must have Calendar API access) |
| `GOOGLE_MEET_CALENDAR_ID` | Calendar ID to create conferences under |

Users must also link their Google account via **Profile → Link Google Account** so Alga can create conferences on their behalf.

### API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/google-meet` | `incidents:read` | Get the incident's Google Meet conference |
| `POST` | `/api/v1/incidents/{id}/google-meet` | `incidents:command` | Provision a conference manually |

## Multi-Agent Coordination

When AI agents are assigned ICS roles, they collaborate through a **task-driven coordination model**:

1. The **Commander agent** decomposes the incident into typed tasks (`investigate`, `communicate`, `verify`, `mitigate`) via `alga_dispatch_task`
2. Each dispatched task **wakes the assigned agent** through the `coordination_task_dispatched` SSE event
3. The agent acts on the task and **completes it** via `alga_complete_task` with a typed result
4. The commander tracks progress via `alga_list_tasks` and writes the conclusion via `alga_synthesize_findings`

This replaces ad-hoc @mention coordination with structured, typed work units — reducing cross-talk and ping-pong between agents.

::: tip Resolution Requires Structured Artifacts
An incident can only be resolved when the commander supplies five resolution documents: `summary`, `impact_assessment`, `actions_taken`, `root_cause`, and `resolution`. The `root_cause` and `resolution` sections are independently mandatory. The commander stages them with `alga_set_incident_resolution_docs` or supplies them inline to `alga_resolve_incident`.
:::

See the [Hermes](/integrations/hermes#multi-agent-incident-coordination) integration guide for detailed coordination guidance.

## Best Practices

- **One IC at a time** — use formal handoff for shift changes; never leave the IC role vacant
- **Include handoff notes** — open issues, ongoing actions, and context for the incoming IC
- **Formally end roles** when no longer needed to keep the role board clean
- **Assign communicators early** — stakeholders need updates fast; don't wait until the incident is resolved to communicate
- **Let agents handle routine** — if you have AI agents, assign them responder or communicator roles for parallel investigation and status publishing

## See Also

- [Lifecycle & States](/incident-management/lifecycle) — incident state machine and transitions
- [Incident Coordination](/incident-management/coordination) — communication channels during incidents
- [AI Investigation](/core-features/investigation) — how AI agents participate in incident response
- [Hermes Integration](/integrations/hermes) — agent coordination protocol details
