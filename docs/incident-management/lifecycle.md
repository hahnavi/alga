---
title: Incident Lifecycle & States
description: The incident state machine — detected, triaging, active, mitigated, resolved, closed — with transitions, automatic actions, and API endpoints.
---

# Incident Lifecycle & States

Incidents follow a strict state machine that coordinates team response from creation through closure.

## State Machine

```
detected → triaging → active → mitigated → resolved → closed
             ↘ active ↗
active → cancelled
mitigated → active (reopened)
resolved → active (reopened)
closed → active (reopened)
```

## States

| Status | Description | Trigger |
|--------|-------------|---------|
| `detected` | Initial creation, awaiting triage | Auto-created by triage worker or manually |
| `triaging` | Undergoing triage assessment | `POST /api/v1/incidents/{id}/begin-triage` |
| `active` | Acknowledged, active response | `POST /api/v1/incidents/{id}/acknowledge` or promote |
| `mitigated` | Root cause fixed, monitoring | `POST /api/v1/incidents/{id}/mitigate` |
| `resolved` | Fully resolved | `POST /api/v1/incidents/{id}/resolve` |
| `closed` | Post-mortem complete | `POST /api/v1/incidents/{id}/close` |
| `cancelled` | False alarm | `POST /api/v1/incidents/{id}/cancel` |

> **Creation nuance:** Auto-created incidents (generated from correlated critical alerts) are created in `detected` and then immediately auto-transition to `active` (auto-acknowledged) — they do not await manual triage. Manually-created incidents start in `detected` and await triage or direct acknowledgment. The state machine itself (detected → triaging → active → mitigated → resolved → closed, plus `cancelled`) is unchanged.

## Transitions

### Detected → Triaging (Begin Triage)

When triage begins on a detected incident:
- Status transitions to `triaging`
- Triage assessment is initiated
- Timeline entry recorded

### Triaging → Active (Promote)

When triage determines the incident is valid:
- Incident is promoted to `active`
- Updates service status to `degraded` or `major_outage`
- Notifies on-call responders
- SLA clocks start

### Detected → Active (Acknowledge)

When the incident is acknowledged directly from detected:
- Transitions to `active` (skips triaging)
- Updates service status to `degraded` or `major_outage`
- Notifies on-call responders

### Active → Mitigated

When the root cause is fixed:
- SLA resolution clock stops
- Service status begins recovery
- Timeline entry recorded automatically

### Mitigated → Resolved

When the incident is fully resolved:
- SLA metrics computed (MTTA, MTTR, MTTM)
- Service status returns to `operational`
- Post-mortem can be created

### Resolved → Closed

After post-mortem is published and action items are tracked:
- Final metrics recorded
- Incident marked as closed

### Reopen

Mitigated, resolved, or closed incidents can be reopened:
- Service status reverts to impacted state
- SLA clocks restart
- Escalation resumes if configured

### Cancel

Cancel is available from `detected`, `triaging`, and `active` states. Cancelled incidents are treated as false alarms:
- Service status returns to `operational`
- Linked alerts are unlinked
- No post-mortem required

## Automatic Actions

Each transition triggers:

| Transition | Automatic Actions |
|-----------|-------------------|
| → `triaging` | Record timeline entry, begin assessment |
| → `active` | Update service status, notify responders, start SLA clocks |
| → `mitigated` | Compute MTTM, update service status, notify stakeholders |
| → `resolved` | Compute MTTR, restore service status, enable post-mortem creation |
| → `closed` | Compute final metrics, archive incident |
| → `cancelled` | Restore service status, clear escalation state |
| → `reopened` | Restart SLA, re-escalate, update service status |

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
- [ICS Roles](/incident-management/ics-roles) — ICS role assignments and handoffs
- [SLA Tracking](/incident-management/sla) — SLA configuration and breach detection
- [Post-Mortems](/incident-management/post-mortems) — structured post-incident review
