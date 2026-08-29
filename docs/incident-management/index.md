---
title: Incident Management
description: Alga's incident lifecycle — ICS command roles, SLA tracking, automated escalation, coordination streams, post-mortems, and the agent investigation API.
---

# Incidents

Alga includes a full incident management system that coordinates alerts, investigations, and team response with SLA tracking and automated escalation.

## Incident Lifecycle

Incidents follow a state machine from creation through closure:

```
detected → triaging → active → mitigated → resolved → closed
active → cancelled (terminal)
```

`resolved`, `closed`, and `cancelled` are terminal states. Status transitions use optimistic concurrency (`WHERE status IN fromStatuses`), so concurrent transitions fail with a conflict instead of corrupting state. Deletion is a soft delete via a `deleted_at` tombstone.

See [Lifecycle & States](/incident-management/lifecycle) for the full state machine, transition triggers, and automatic actions.

## Key Concepts

- **Detected State**: Incidents start in `detected` state, awaiting triage
- **Triage Flow**: `begin-triage` moves a detected incident to `triaging`; `promote` moves a triaging incident to `active`
- **SLA Targets**: `sla_target_respond_at` and `sla_target_resolve_at` are computed from the service SLA config at creation
- **Escalation**: Policy-driven multi-tier escalation. The SLA worker triggers escalation on a response breach; acknowledgement stops escalation
- **ICS Role Assignments**: Formal ICS (Incident Command System) roles — commander, communicator, responder — for structured command. Users and agents can hold roles. See [ICS Roles](/incident-management/ics-roles)
- **Coordination**: Real-time coordination messages and public status updates — see [Coordination](/incident-management/coordination)
- **Incident Documents**: Section-based collaborative documents (current status, impact, root cause, resolution, etc.) with per-section versioning
- **IC Handoffs**: Structured handoff process for transferring on-call responsibility — see [Handoffs](/incident-management/handoffs)
- **Alert Linking**: Alerts can be linked to and unlinked from incidents; resolving an incident cascades `resolved` to linked firing alerts
- **Incident Investigations**: Investigations scoped to an incident with their own lifecycle; only one active investigation per incident is enforced

## Creating Incidents

Incidents can be created:

1. **Manually** via `POST /api/v1/incidents` or the Incidents page
2. **By promotion** — `promote` creates an incident from an alert investigation/triage flow

## Incident Roles

Alga uses the Incident Command System (ICS) for structured incident response — Incident Commander, Communicator, and Responder. Both users and agents can be assigned roles. See [ICS Roles](/incident-management/ics-roles).

## API Endpoints

Incident routes are addressed by `incident_number` (the human-readable unique number), not the internal UUID.

### Incident Management

| Method   | Path                     | Auth    | Permission         | Description                                                                         |
| -------- | ------------------------ | ------- | ------------------ | ----------------------------------------------------------------------------------- |
| `GET`    | `/api/v1/incidents`      | Session | `incidents:read`   | List incidents (filters: status, severity, service_id, commander_id, search, dates) |
| `POST`   | `/api/v1/incidents`      | Session | `incidents:write`  | Create manual incident                                                              |
| `GET`    | `/api/v1/incidents/{id}` | Session | `incidents:read`   | Get incident with timeline, roles, linked items                                     |
| `PATCH`  | `/api/v1/incidents/{id}` | Session | `incidents:write`  | Update (title, description, severity, custom_fields)                                |
| `DELETE` | `/api/v1/incidents/{id}` | Session | `incidents:delete` | Soft-delete incident (`deleted_at` tombstone)                                       |

### Incident Actions

| Method | Path                                 | Auth    | Permission          | Description                                                                     |
| ------ | ------------------------------------ | ------- | ------------------- | ------------------------------------------------------------------------------- |
| `POST` | `/api/v1/incidents/{id}/acknowledge` | Session | `incidents:command` | Acknowledge (stops escalation, sets `sla_acknowledged_at`)                      |
| `POST` | `/api/v1/incidents/{id}/mitigate`    | Session | `incidents:command` | Mark mitigated (sets `mitigated_at`)                                            |
| `POST` | `/api/v1/incidents/{id}/resolve`     | Session | `incidents:command` | Mark resolved (sets `resolved_at`/`sla_resolved_at`, cascades to linked alerts) |
| `POST` | `/api/v1/incidents/{id}/close`       | Session | `incidents:command` | Mark closed (sets `closed_at`)                                                  |
| `POST` | `/api/v1/incidents/{id}/reopen`      | Session | `incidents:command` | Reopen a resolved/mitigated/closed incident                                     |
| `POST` | `/api/v1/incidents/{id}/cancel`      | Session | `incidents:command` | Cancel (terminal, false alarm)                                                  |
| `POST` | `/api/v1/incidents/{id}/escalate`    | Session | `incidents:command` | Manual escalation trigger                                                       |

### Triage

| Method | Path                                  | Auth    | Permission          | Description                               |
| ------ | ------------------------------------- | ------- | ------------------- | ----------------------------------------- |
| `POST` | `/api/v1/incidents/{id}/begin-triage` | Session | `incidents:command` | Begin triage (`detected` → `triaging`)    |
| `POST` | `/api/v1/incidents/{id}/promote`      | Session | `incidents:command` | Promote to active (`triaging` → `active`) |

### Timeline

| Method | Path                              | Auth    | Permission        | Description               |
| ------ | --------------------------------- | ------- | ----------------- | ------------------------- |
| `GET`  | `/api/v1/incidents/{id}/timeline` | Session | `incidents:read`  | Get structured timeline   |
| `POST` | `/api/v1/incidents/{id}/timeline` | Session | `incidents:write` | Add manual timeline entry |

Timeline entries carry `event_type`, `actor_id`, `actor_type` (`system`/`user`/`agent`), `message`, `metadata`, and an optional `ics_event_type`.

### Linked Alerts

| Method   | Path                                           | Auth    | Permission        | Description        |
| -------- | ---------------------------------------------- | ------- | ----------------- | ------------------ |
| `GET`    | `/api/v1/incidents/{id}/alerts`                | Session | `incidents:read`  | List linked alerts |
| `POST`   | `/api/v1/incidents/{id}/alerts`                | Session | `incidents:write` | Link alert         |
| `DELETE` | `/api/v1/incidents/{id}/alerts/{alert_number}` | Session | `incidents:write` | Unlink alert       |

### Incident Investigations

| Method  | Path                                          | Auth    | Permission        | Description                         |
| ------- | --------------------------------------------- | ------- | ----------------- | ----------------------------------- |
| `GET`   | `/api/v1/incidents/{id}/investigations`       | Session | `incidents:read`  | List investigations under incident  |
| `POST`  | `/api/v1/incidents/{id}/investigations`       | Session | `incidents:write` | Create investigation under incident |
| `PATCH` | `/api/v1/incident-investigations/{id}/assign` | Session | `incidents:write` | Assign an incident investigation    |

Incident investigations follow `pending → assigned → investigating → complete/cancelled/paused/coordinating`. Only one active investigation per incident is enforced, and parent/child investigations are supported.

### Coordination

| Method | Path                                           | Auth    | Permission        | Description                |
| ------ | ---------------------------------------------- | ------- | ----------------- | -------------------------- |
| `GET`  | `/api/v1/incidents/{id}/coordination/messages` | Session | `incidents:read`  | List coordination messages |
| `POST` | `/api/v1/incidents/{id}/coordination/messages` | Session | `incidents:write` | Add coordination message   |

### Status Updates

| Method | Path                                    | Auth    | Permission          | Description          |
| ------ | --------------------------------------- | ------- | ------------------- | -------------------- |
| `GET`  | `/api/v1/incidents/{id}/status-updates` | Session | `incidents:read`    | List status updates  |
| `POST` | `/api/v1/incidents/{id}/status-updates` | Session | `incidents:command` | Create status update |

### Incident Document (ICS)

| Method | Path                                            | Auth    | Permission        | Description                               |
| ------ | ----------------------------------------------- | ------- | ----------------- | ----------------------------------------- |
| `GET`  | `/api/v1/incidents/{id}/ics/document`           | Session | `incidents:read`  | Get all document sections                 |
| `PUT`  | `/api/v1/incidents/{id}/ics/document/{section}` | Session | `incidents:write` | Update a single section (version-checked) |

### ICS Roles

| Method   | Path                                        | Auth    | Permission          | Description                |
| -------- | ------------------------------------------- | ------- | ------------------- | -------------------------- |
| `GET`    | `/api/v1/incidents/{id}/ics/roles`          | Session | `incidents:read`    | List ICS role assignments  |
| `POST`   | `/api/v1/incidents/{id}/ics/roles`          | Session | `incidents:command` | Assign ICS role            |
| `PATCH`  | `/api/v1/incidents/{id}/ics/roles/{roleId}` | Session | `incidents:command` | Update ICS role assignment |
| `DELETE` | `/api/v1/incidents/{id}/ics/roles/{roleId}` | Session | `incidents:command` | End ICS role assignment    |

### Slack Incident Channels

| Method   | Path                                   | Auth    | Permission          | Description                    |
| -------- | -------------------------------------- | ------- | ------------------- | ------------------------------ |
| `POST`   | `/api/v1/incidents/{id}/slack-channel` | Session | `incidents:command` | Create dedicated Slack channel |
| `DELETE` | `/api/v1/incidents/{id}/slack-channel` | Session | `incidents:command` | Delete/unlink Slack channel    |

### Google Meet

| Method   | Path                                 | Auth    | Permission          | Description              |
| -------- | ------------------------------------ | ------- | ------------------- | ------------------------ |
| `POST`   | `/api/v1/incidents/{id}/google-meet` | Session | `incidents:command` | Create Google Meet space |
| `DELETE` | `/api/v1/incidents/{id}/google-meet` | Session | `incidents:command` | Unlink Google Meet space |

### Metrics

| Method | Path                        | Auth    | Permission       | Description                                          |
| ------ | --------------------------- | ------- | ---------------- | ---------------------------------------------------- |
| `GET`  | `/api/v1/incidents/metrics` | Session | `incidents:read` | Aggregate metrics (MTTA, MTTR, MTTM, SLA compliance) |

## SLA Tracking

Alga computes SLA deadlines at creation and a background SLA worker sweeps for breaches — see [SLA Tracking](/incident-management/sla) for configuration and breach detection.

## Agent API Endpoints

| Method  | Path                                    | Auth   | Description                                       |
| ------- | --------------------------------------- | ------ | ------------------------------------------------- |
| `GET`   | `/api/v1/agent/incidents/{id}`          | Bearer | Get incident context                              |
| `GET`   | `/api/v1/agent/incidents/{id}/timeline` | Bearer | Get incident timeline                             |
| `POST`  | `/api/v1/agent/incidents/{id}/timeline` | Bearer | Add timeline entry                                |
| `PATCH` | `/api/v1/agent/incidents/{id}`          | Bearer | Update incident (requires investigate capability) |

## See Also

- [Lifecycle & States](/incident-management/lifecycle) — state machine and transitions
- [ICS Roles](/incident-management/ics-roles) — ICS role assignments and handoffs
- [Coordination](/incident-management/coordination) — coordination messages, tasks, and status updates
- [Handoffs](/incident-management/handoffs) — on-call handoff process
- [SLA Tracking](/incident-management/sla) — SLA configuration and breach detection
- [Post-Mortems](/incident-management/post-mortems) — structured post-incident review
