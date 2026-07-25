---
title: Incident Lifecycle & States
description: The incident state machine — detected, triaging, active, mitigated, resolved, closed, cancelled — with transitions, automatic timestamps, and API endpoints.
---

# Incident Lifecycle & States

Incidents follow a strict state machine that coordinates team response from creation through closure.

## State Machine

```
detected → triaging → active → mitigated → resolved → closed
detected → active (acknowledge)
detected/active → mitigated
detected/active/mitigated → resolved
mitigated/resolved/closed → active (reopen)
detected/active → cancelled
```

`resolved`, `closed`, and `cancelled` are terminal states. Deletion is a soft delete via a `deleted_at` tombstone — rows are never hard-deleted.

## States

| Status | Description | Typical Trigger |
|--------|-------------|-----------------|
| `detected` | Initial creation, awaiting triage or acknowledgement | Manual creation or promotion from an alert investigation |
| `triaging` | Undergoing triage assessment | `POST /api/v1/incidents/{id}/begin-triage` |
| `active` | Acknowledged, active response underway | `acknowledge`, or `promote` from triaging |
| `mitigated` | Contained/fix in place, monitoring | `POST /api/v1/incidents/{id}/mitigate` |
| `resolved` | Fully resolved (terminal) | `POST /api/v1/incidents/{id}/resolve` |
| `closed` | Finalized after resolution (terminal) | `POST /api/v1/incidents/{id}/close` |
| `cancelled` | False alarm (terminal) | `POST /api/v1/incidents/{id}/cancel` |

## Optimistic Concurrency

Every transition goes through `TransitionIncidentStatus`, which guards the update with `WHERE status IN (fromStatuses)`. If the incident is no longer in an allowed source state, the update affects zero rows and the API returns a conflict (`incident status changed concurrently`) instead of corrupting state. This makes concurrent commands safe.

## Transitions

Each row lists the source states accepted by the action and the resulting state.

| Action | From | To | Notes |
|--------|------|----|-------|
| Begin Triage | `detected` | `triaging` | Initializes the incident document sections |
| Promote | `triaging` | `active` | Propagates service status |
| Acknowledge | `detected` | `active` | Sets `sla_acknowledged_at`, stops escalation |
| Mitigate | `detected`, `active` | `mitigated` | Sets `mitigated_at`, propagates service status |
| Resolve | `detected`, `active`, `mitigated` | `resolved` | Requires resolution docs (see below); cascades `resolved` to linked firing alerts |
| Close | `resolved` | `closed` | Sets `closed_at` |
| Reopen | `mitigated`, `resolved`, `closed` | `active` | Returns incident to active response |
| Cancel | `detected`, `active` | `cancelled` | Terminal false-alarm state |

### Resolution Requirements

`resolve` is rejected unless the incident has a non-empty `summary` and the following incident-document sections are filled in:

- `impact_assessment`
- `root_cause`
- `resolution`

If any are missing, the API returns a validation error listing the missing fields.

## Automatic Timestamps

`applyStatusTimestamps` stamps lifecycle fields as each transition lands:

| Transition To | Timestamp(s) Set |
|---------------|------------------|
| `triaging` | `triaged_at` |
| `active` | `sla_acknowledged_at` |
| `mitigated` | `mitigated_at` |
| `resolved` | `resolved_at` and `sla_resolved_at` |
| `closed` | `closed_at` |

These timestamps drive the SLA metrics (MTTA, MTTR, MTTM) reported by the metrics API.

## Side Effects

Beyond timestamps, transitions trigger:

- **Timeline entries** — each action records a structured timeline entry (with an ICS event type for triage/promote/document changes)
- **Service status propagation** — promote, mitigate, resolve, and reopen update the affected service status
- **Alert cascade** — resolving an incident cascades `resolved` to linked firing alerts
- **Audit events** — every transition writes a fire-and-forget audit event
- **Incident channel updates** — when Slack incident channels are enabled, status changes are posted to the incident channel
- **SLA escalation** — the SLA worker triggers escalation on a response breach; acknowledgement stops it

## API Endpoints

| Action | Method | Path | Permission |
|--------|--------|------|------------|
| Begin Triage | `POST` | `/api/v1/incidents/{id}/begin-triage` | `incidents:command` |
| Promote | `POST` | `/api/v1/incidents/{id}/promote` | `incidents:command` |
| Acknowledge | `POST` | `/api/v1/incidents/{id}/acknowledge` | `incidents:command` |
| Mitigate | `POST` | `/api/v1/incidents/{id}/mitigate` | `incidents:command` |
| Resolve | `POST` | `/api/v1/incidents/{id}/resolve` | `incidents:command` |
| Close | `POST` | `/api/v1/incidents/{id}/close` | `incidents:command` |
| Reopen | `POST` | `/api/v1/incidents/{id}/reopen` | `incidents:command` |
| Cancel | `POST` | `/api/v1/incidents/{id}/cancel` | `incidents:command` |
| Escalate | `POST` | `/api/v1/incidents/{id}/escalate` | `incidents:command` |

## See Also

- [Incident Overview](/incident-management/) — creation, linking, and management
- [ICS Roles](/incident-management/ics-roles) — ICS role assignments
- [SLA Tracking](/incident-management/sla) — SLA configuration and breach detection
- [Post-Mortems](/incident-management/post-mortems) — structured post-incident review
