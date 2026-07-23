---
title: On-Call Handoffs
description: Structured shift handoffs with notes, active incident summaries, and acknowledgment — plus pager-load metrics for balancing the load.
---

# On-Call Handoffs

Alga tracks on-call shift handoffs with structured notes and acknowledgment to ensure continuity during shift transitions.

## How Handoffs Work

```
Shift Ending → Outgoing Notes → Handoff Created → Incoming Acknowledges → Shift Active
```

1. An on-call shift ends and the next responder takes over
2. The outgoing on-call creates a handoff with outgoing notes
3. The incoming on-call acknowledges and adds incoming notes
4. Handoff is recorded in the audit log for traceability

## Handoff Record

| Field | Description |
|-------|-------------|
| `schedule_id` | The on-call schedule this handoff belongs to |
| `outgoing_user_id` | User ending their shift |
| `incoming_user_id` | User starting their shift |
| `outgoing_notes` | Notes from the outgoing on-call (open issues, context, pending items) |
| `incoming_notes` | Acknowledgment and notes from the incoming on-call |
| `status` | `pending` or `acknowledged` |
| `shift_start` | When the incoming shift starts |
| `shift_end` | When the incoming shift ends |

## API Endpoints

### Handoff Management

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/handoffs` | Session | `oncall:read` | List handoffs |
| `GET` | `/api/v1/on-call/handoffs/{id}` | Session | `oncall:read` | Get handoff details |
| `GET` | `/api/v1/on-call/handoffs/pending` | Session | `oncall:read` | List pending handoffs for current user |
| `PATCH` | `/api/v1/on-call/handoffs/{id}/notes` | Session | `oncall:write` | Save handoff notes |
| `POST` | `/api/v1/on-call/handoffs/{id}/acknowledge` | Session | `oncall:write` | Acknowledge handoff |

### Example: Creating Outgoing Notes

```sh
curl -X PATCH http://localhost:8080/api/v1/on-call/handoffs/{id}/notes \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "outgoing_notes": "Two open investigations: DB conn pool (#4512) and API latency (#4513). DB team is engaged on the pool issue. No active incidents."
  }'
```

### Example: Acknowledging a Handoff

```sh
curl -X POST http://localhost:8080/api/v1/on-call/handoffs/{id}/acknowledge \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "incoming_notes": "Reviewed both investigations. DB team expects fix in 30 min. I will monitor."
  }'
```

## Pager Load Metrics

Track on-call burden with shift-level metrics to identify overloaded rotations and ensure fair load distribution:

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/metrics` | Session | `oncall:read` | Pager load metrics per shift |

### Available Metrics

| Metric | Description |
|--------|-------------|
| `alerts_received` | Total alerts that fired during the shift |
| `alerts_acknowledged` | Alerts acknowledged within the shift |
| `alerts_resolved` | Alerts resolved during the shift |
| `alerts_missed` | Alerts not acknowledged before shift ended |
| `avg_ack_time_seconds` | Average time to acknowledge an alert |

Metrics can be filtered by `schedule_id`, `user_id`, and date range.

## Best Practices

- **Always write outgoing notes** with open issues, ongoing investigations, and any context the next on-call needs
- **Acknowledge handoffs promptly** to confirm continuity and signal readiness
- **Review pager load metrics** regularly to identify overloaded on-call rotations
- **Use overrides** for planned absences rather than informal swaps — overrides are tracked and auditable
- **Escalate early** if the incoming on-call does not acknowledge before the shift starts
- **Keep notes concise** — focus on actionable items, not exhaustive incident histories

## See Also

- [On-Call Schedules](/on-call/schedules) — creating and managing rotation schedules
- [Escalation Policies](/on-call/escalation-policies) — configuring multi-tier escalation
- [Notification Preferences](/on-call/notification-preferences) — per-user notification channels
