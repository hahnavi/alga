---
title: ICS Incident Command System
description: Formal Incident Command System roles — Incident Commander, Communicator, Responder — with user/agent assignees, role hierarchy, incident documents, and Google Meet war rooms.
---

# ICS Incident Command System

Alga implements the Incident Command System (ICS) for structured incident response — a hierarchical command structure with formal role assignments and war-room coordination. Every incident can have a clearly defined chain of command with specific responsibilities for each role.

## Why ICS?

ICS is the same framework used by emergency responders worldwide, adapted for technology incidents. It solves three problems during high-stress incidents:

1. **Who's in charge?** — An Incident Commander owns incident direction and final calls
2. **Who does what?** — Roles separate command (decision-making), communications (status updates), and response (hands-on fix)
3. **How do we hand off?** — Roles can be ended and reassigned as responders rotate

## ICS Roles

| Role Type             | Label              | Responsibility                                                                        |
| --------------------- | ------------------ | ------------------------------------------------------------------------------------- |
| `incident_commander`  | Incident Commander | Owns incident direction, escalation decisions, final calls, and documentation quality |
| `communications_lead` | Communicator       | Owns status updates, stakeholder communication, summaries, and human-facing messages  |
| `responder`           | Responder          | Owns investigation, mitigation, evidence gathering, and technical recovery work       |

Each role maps to a required agent capability: `incident_commander` → command, `communications_lead` → communicate, `responder` → investigate. Agents assigned a role must hold the matching capability.

## Role Assignments

An ICS role assignment (`ICSRoleAssignment`) tracks:

| Field                     | Description                                                                       |
| ------------------------- | --------------------------------------------------------------------------------- |
| `role_type`               | `incident_commander`, `communications_lead`, or `responder`                       |
| `assignee_type`           | `user` or `agent`                                                                 |
| `status`                  | `active` or `ended`                                                               |
| `scope_description`       | Optional free-text scope (also used for custom role scoping)                      |
| `ended_reason`            | Why the role ended (`replaced`, `incident_resolved`, `assigned`, `agent_offline`) |
| `started_at` / `ended_at` | Role tenure                                                                       |

Assignments form a parent/child hierarchy via a self-referential `parent` edge, so a commander can have responder or communicator roles nested beneath it. Agents hold roles through the `agent_token` edge; users through the `user` edge.

## Assigning ICS Roles

### Via the API

Assign to a user:

```bash
curl -X POST http://localhost:8080/api/v1/incidents/{id}/ics/roles \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "role_type": "incident_commander",
    "user_id": "550e8400-e29b-41d4-a716-446655440000"
  }'
```

Assign to an agent (provide `agent_token_id` instead of `user_id`; the two are mutually exclusive):

```bash
curl -X POST http://localhost:8080/api/v1/incidents/{id}/ics/roles \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "role_type": "responder",
    "agent_token_id": "770e8400-e29b-41d4-a716-446655440002",
    "scope_description": "Investigate database connection pool exhaustion"
  }'
```

## Auto-Assignment

When an incident is processed, the incident worker auto-assigns the Incident Commander role by resolving the on-call user for the affected service (via its on-call schedule and escalation policy). If an on-call user is found, they become IC and a timeline entry is recorded. If none is found, the IC role is left for manual assignment.

## Ending and Updating Roles

Roles are ended (not deleted) to preserve the role history. Ending sets `status=ended` with an `ended_reason` and stamps `ended_at`. Common end reasons:

- `replaced` — a new assignee took over the role
- `incident_resolved` — the incident moved to a terminal state
- `assigned` — superseded by a new assignment
- `agent_offline` — the assigned agent went offline

## Incident Document

Each incident has a section-based collaborative document used for shared note-taking during response. Sections are version-tracked per section and record the user who last updated them.

| Section             | Purpose                                    |
| ------------------- | ------------------------------------------ |
| `current_status`    | Current state of the incident              |
| `impact_assessment` | What is affected, severity, blast radius   |
| `actions_taken`     | Remediation steps and outcomes             |
| `open_questions`    | Unresolved questions                       |
| `resources`         | Links and references                       |
| `timeline_summary`  | Condensed event timeline                   |
| `root_cause`        | Technical root cause (required to resolve) |
| `resolution`        | How it was resolved (required to resolve)  |

`impact_assessment`, `root_cause`, and `resolution` (plus the incident `summary`) must be filled in before the incident can be resolved.

### Document API

| Method | Path                                            | Permission        | Description                        |
| ------ | ----------------------------------------------- | ----------------- | ---------------------------------- |
| `GET`  | `/api/v1/incidents/{id}/ics/document`           | `incidents:read`  | Get all document sections          |
| `PUT`  | `/api/v1/incidents/{id}/ics/document/{section}` | `incidents:write` | Update a section (version-checked) |

## Role Management API

| Method   | Path                                        | Permission          | Description                     |
| -------- | ------------------------------------------- | ------------------- | ------------------------------- |
| `GET`    | `/api/v1/incidents/{id}/ics/roles`          | `incidents:read`    | List ICS role assignments       |
| `POST`   | `/api/v1/incidents/{id}/ics/roles`          | `incidents:command` | Assign ICS role (user or agent) |
| `PATCH`  | `/api/v1/incidents/{id}/ics/roles/{roleId}` | `incidents:command` | Update ICS role assignment      |
| `DELETE` | `/api/v1/incidents/{id}/ics/roles/{roleId}` | `incidents:command` | End ICS role assignment         |

## Google Meet War Rooms

When an incident is active, the team needs a place to coordinate in real time. Alga can provision a **Google Meet conference space** per incident for war-room-style voice/video coordination. The Meet space name and conference URL are stored on the incident; unlinking clears both.

### Enabling War Rooms

Set these environment variables:

| Variable                       | Description                                                                                                                                |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `GOOGLE_MEET_ENABLED`          | Set to `true` to enable war-room provisioning                                                                                              |
| `GOOGLE_MEET_CREDENTIALS_PATH` | Path to a service-account JSON with the Meet API enabled and domain-wide delegation for `https://www.googleapis.com/auth/meet.space.admin` |
| `GOOGLE_MEET_AUTO_CREATE`      | Set to `true` to auto-create a Meet space per incident                                                                                     |

### API Endpoints

| Method   | Path                                 | Permission          | Description                          |
| -------- | ------------------------------------ | ------------------- | ------------------------------------ |
| `POST`   | `/api/v1/incidents/{id}/google-meet` | `incidents:command` | Create a Meet space for the incident |
| `DELETE` | `/api/v1/incidents/{id}/google-meet` | `incidents:command` | Unlink the Meet space                |

## Multi-Agent Coordination

When AI agents are assigned ICS roles, they collaborate through the **coordination thread**: the commander delegates by @mentioning responder/communicator agents (a mention activates them), agents publish milestone status updates, and responders hand work back with structured `alga_post_handoff` messages. See [Coordination](/incident-management/coordination) for the message model.

## Best Practices

- **Keep a single active commander** — end the outgoing commander role before or as you assign a new one
- **Formally end roles** when no longer needed to keep the role board clean and the history accurate
- **Assign communicators early** — stakeholders need updates fast; don't wait until the incident is resolved to communicate
- **Let agents handle routine work** — assign agents responder or communicator roles for parallel investigation and status publishing (they must hold the matching capability)

## See Also

- [Lifecycle & States](/incident-management/lifecycle) — incident state machine and transitions
- [Incident Coordination](/incident-management/coordination) — coordination messages and status updates
- [Handoffs](/incident-management/handoffs) — on-call shift handoffs
- [AI Investigation](/core-features/investigation) — how AI agents participate in incident response
