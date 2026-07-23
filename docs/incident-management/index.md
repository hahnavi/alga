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
              ↘ active ↗
active → cancelled
mitigated → active (reopened)
resolved → active (reopened)
closed → active (reopened)
```

See [Lifecycle & States](/incident-management/lifecycle) for the full state machine, transition triggers, and automatic actions.

## Key Concepts

- **Detected State**: Incidents start in `detected` state, awaiting triage
- **Triage Flow**: Detected incidents undergo triage before being promoted to active — see [Triage](#triage)
- **SLA Targets**: Computed from severity or service config (`sla_target_respond_at`, `sla_target_resolve_at`)
- **Escalation**: Policy-driven multi-tier escalation with configurable delays. Acknowledgement stops escalation
- **Service Status**: Incident transitions update affected service status (operational → degraded → major_outage)
- **Dependency Cascade**: Incidents on a service auto-create timeline entries for dependent services
- **ICS Role Assignments**: Formal ICS (Incident Command System) roles for structured command — see [ICS Roles](/incident-management/ics-roles)
- **Coordination Threads**: Real-time coordination messages between incident responders — see [Coordination](/incident-management/coordination)
- **Incident Documents**: Structured documents attached to incidents for runbooks, notes, and reference material
- **Status Updates**: Formal status updates posted during incident response for stakeholder communication
- **IC Handoffs**: Structured handoff process for transferring incident commander responsibility — see [Handoffs](/incident-management/handoffs)

## Creating Incidents

Incidents can be created:
1. **Automatically** via the triage worker when correlated alerts are classified as incident-worthy
2. **Manually** via `POST /api/v1/incidents` or the Incidents page

## Incident Roles

Alga uses the Incident Command System (ICS) for structured incident response — Incident Commander, Comms Lead, and Responder. See [ICS Roles](/incident-management/ics-roles).

## API Endpoints

### Incident Management
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents` | Session | `incidents:read` | List incidents (filters: status, severity, service_id, commander_id, search, dates) |
| `POST` | `/api/v1/incidents` | Session | `incidents:write` | Create manual incident |
| `GET` | `/api/v1/incidents/{id}` | Session | `incidents:read` | Get incident with timeline, roles, linked items |
| `PATCH` | `/api/v1/incidents/{id}` | Session | `incidents:write` | Update (title, description, severity, custom_fields) |
| `DELETE` | `/api/v1/incidents/{id}` | Session | `incidents:delete` | Delete incident |

### Incident Actions
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/acknowledge` | Session | `incidents:command` | Acknowledge (stops escalation, starts SLA clock) |
| `POST` | `/api/v1/incidents/{id}/mitigate` | Session | `incidents:command` | Mark mitigated |
| `POST` | `/api/v1/incidents/{id}/resolve` | Session | `incidents:command` | Mark resolved |
| `POST` | `/api/v1/incidents/{id}/close` | Session | `incidents:command` | Mark closed |
| `POST` | `/api/v1/incidents/{id}/reopen` | Session | `incidents:command` | Reopen resolved/mitigated |
| `POST` | `/api/v1/incidents/{id}/cancel` | Session | `incidents:command` | Cancel (false alarm) |
| `POST` | `/api/v1/incidents/{id}/escalate` | Session | `incidents:command` | Manual escalation trigger |

### Timeline
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/timeline` | Session | `incidents:read` | Get structured timeline |
| `POST` | `/api/v1/incidents/{id}/timeline` | Session | `incidents:write` | Add manual timeline entry |

### Linked Items
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/alerts` | Session | `incidents:read` | List linked alerts |
| `POST` | `/api/v1/incidents/{id}/alerts` | Session | `incidents:write` | Link alert |
| `DELETE` | `/api/v1/incidents/{id}/alerts/{fp}` | Session | `incidents:write` | Unlink alert |
| `GET` | `/api/v1/incidents/{id}/investigations` | Session | `incidents:read` | List investigations under incident |
| `POST` | `/api/v1/incidents/{id}/investigations` | Session | `incidents:write` | Create investigation under incident |

### Coordination
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/coordination` | Session | `incidents:read` | List coordination messages |
| `POST` | `/api/v1/incidents/{id}/coordination` | Session | `incidents:write` | Add coordination message |

### Investigation Thread
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/thread` | Session | `incidents:read` | Get investigation thread |
| `POST` | `/api/v1/incidents/{id}/thread/messages` | Session | `incidents:write` | Add thread message |

### Status Updates
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/status-updates` | Session | `incidents:read` | List status updates |
| `POST` | `/api/v1/incidents/{id}/status-updates` | Session | `incidents:command` | Create status update |

### Incident Document
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/document` | Session | `incidents:read` | Get document |
| `PATCH` | `/api/v1/incidents/{id}/document/{section}` | Session | `incidents:write` | Update section |

### ICS Roles
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/ics/roles` | Session | `incidents:read` | List ICS role assignments |
| `POST` | `/api/v1/incidents/{id}/ics/roles` | Session | `incidents:command` | Assign ICS role |
| `POST` | `/api/v1/incidents/{id}/ics/handoff` | Session | `incidents:command` | IC handoff |
| `POST` | `/api/v1/incidents/{id}/ics/handoff/{handoffId}/acknowledge` | Session | `incidents:command` | Acknowledge handoff |

### Triage
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/begin-triage` | Session | `incidents:command` | Begin triage |
| `POST` | `/api/v1/incidents/{id}/promote` | Session | `incidents:command` | Promote to active |

### Slack Incident Channels
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/slack-channel` | Session | `incidents:command` | Create Slack channel for incident |
| `DELETE` | `/api/v1/incidents/{id}/slack-channel` | Session | `incidents:command` | Unlink Slack channel from incident |

### Metrics
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/metrics` | Session | `incidents:read` | Aggregate metrics (MTTA, MTTR, MTTM, SLA compliance) |

## SLA Tracking

Alga tracks SLA compliance using Valkey sorted sets — see [SLA Tracking](/incident-management/sla) for configuration and breach detection.

## Agent API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/agent/incidents/{id}` | Bearer | Get incident context |
| `POST` | `/api/v1/agent/incidents/{id}/timeline` | Bearer | Add timeline entry |

## See Also

- [Lifecycle & States](/incident-management/lifecycle) — state machine and transitions
- [ICS Roles](/incident-management/ics-roles) — ICS role assignments and handoffs
- [Coordination](/incident-management/coordination) — coordination threads
- [Handoffs](/incident-management/handoffs) — IC handoff process
- [SLA Tracking](/incident-management/sla) — SLA configuration and breach detection
- [Post-Mortems](/incident-management/post-mortems) — structured post-incident review
