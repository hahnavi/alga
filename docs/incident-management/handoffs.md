---
title: On-Call Handoffs
description: Structured shift handoffs with outgoing/incoming notes, incident summaries, and acknowledgment — plus pager-load metrics for balancing the load.
---

# On-Call Handoffs

Alga tracks on-call shift handoffs (`HandoffRecord`) with structured notes and acknowledgment to ensure continuity during shift transitions.

## How Handoffs Work

```
Shift Ending → Outgoing Notes → Handoff Created (pending) → Incoming Acknowledges → acknowledged
```

1. An on-call shift ends and the next responder takes over
2. The outgoing on-call saves outgoing notes (open issues, context, pending items)
3. The incoming on-call saves incoming notes and acknowledges the handoff
4. Each step is recorded in the audit log for traceability

## Handoff Record

| Field | Description |
|-------|-------------|
| `schedule_id` | The on-call schedule this handoff belongs to |
| `outgoing_user_id` | User ending their shift |
| `incoming_user_id` | User starting their shift |
| `handoff_at` | When the handoff occurs |
| `status` | `pending` or `acknowledged` |
| `outgoing_notes` | Notes from the outgoing on-call |
| `incoming_notes` | Notes from the incoming on-call |
| `incoming_acknowledged_at` | When the incoming on-call acknowledged |
| `incident_summary` | Snapshot of active incidents at handoff time |

## API Endpoints

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/handoffs` | Session | `oncall:read` | List handoffs |
| `GET` | `/api/v1/on-call/handoffs/pending` | Session | `oncall:read` | List pending handoffs for the current user |
| `GET` | `/api/v1/on-call/handoffs/{id}` | Session | `oncall:read` | Get handoff details |
| `POST` | `/api/v1/on-call/handoffs/{id}/notes` | Session | `oncall:write` | Save handoff notes |
| `POST` | `/api/v1/on-call/handoffs/{id}/acknowledge` | Session | `oncall:write` | Acknowledge handoff |

### Saving Notes

The notes endpoint takes a `field` (`outgoing_notes` or `incoming_notes`) and the `notes` text. Ownership is enforced: only the outgoing user can save outgoing notes, and only the incoming user can save incoming notes.

```sh
curl -X POST http://localhost:8080/api/v1/on-call/handoffs/{id}/notes \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "field": "outgoing_notes",
    "notes": "Two open investigations: DB conn pool (#4512) and API latency (#4513). DB team is engaged on the pool issue. No active incidents."
  }'
```

### Acknowledging a Handoff

Only the incoming user can acknowledge. Acknowledging sets the status to `acknowledged` and stamps `incoming_acknowledged_at`. Save incoming notes first via the notes endpoint.

```sh
curl -X POST http://localhost:8080/api/v1/on-call/handoffs/{id}/acknowledge \
  -H "Authorization: Bearer $TOKEN"
```

## Pager Load Metrics

Track on-call burden with shift-level metrics to identify overloaded rotations and ensure fair load distribution:

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/metrics` | Session | `oncall:read` | Pager load metrics per shift |

Metrics can be filtered by `schedule_id`, `user_id`, and date range.

## Best Practices

- **Always write outgoing notes** with open issues, ongoing investigations, and any context the next on-call needs
- **Acknowledge handoffs promptly** to confirm continuity and signal readiness
- **Review pager load metrics** regularly to identify overloaded on-call rotations
- **Use overrides** for planned absences rather than informal swaps — overrides are tracked and auditable
- **Keep notes concise** — focus on actionable items, not exhaustive incident histories

## See Also

- [On-Call Schedules](/on-call/schedules) — creating and managing rotation schedules
- [Escalation Policies](/on-call/escalation-policies) — configuring multi-tier escalation
- [ICS Roles](/incident-management/ics-roles) — incident command role assignments
