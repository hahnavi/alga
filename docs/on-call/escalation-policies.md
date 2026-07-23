---
title: Escalation Policies
description: Multi-tier escalation policies with configurable delays, looping, per-level channel selection, and user, team, or on-call schedule targets.
---

# Escalation Policies

Escalation policies define **who gets notified, in what order, and how fast** when an incident isn't acknowledged. Alga's policies are multi-tier with configurable delays, looping, and per-level channel selection — so you can page level 1 by Slack, escalate to level 2 by SMS, and loop until someone responds.

## How Escalation Works

When an incident is created on a service, Alga loads that service's escalation policy and begins the escalation timer:

```
Incident created
      │
      ▼
 Level 1 notified (delay = 0s)
      │
      │  Not acknowledged within delay?
      ▼
 Level 2 notified (delay = 5m)
      │
      │  Not acknowledged within delay?
      ▼
 Level 3 notified (delay = 15m)
      │
      │  Still not acknowledged?
      ▼
 Loop back to Level 1 (up to repeat_count times)
      │
      ▼
 Policy exhausted — escalation stops
```

**Acknowledgement stops escalation immediately.** The moment any responder acknowledges the incident — via the UI, Slack, or the API — all pending escalations are cancelled.

## Policy Structure

Each escalation policy has multiple **levels** triggered sequentially. Each level specifies:

- **Delay** — wait time before escalating to this level (e.g., `0s` for level 1, `5m` for level 2)
- **Targets** — who gets notified at this level (users, teams, or on-call schedules)
- **Notify Channels** — which channels to use at this level (e.g., `["in_app", "email"]`, `["slack", "voice"]`)

### Example: Three-Tier Policy

A common pattern — page the primary on-call immediately, escalate to the secondary after 5 minutes, then pull in the team lead after 15 minutes:

```json
{
  "name": "payments-escalation",
  "repeat_count": 3,
  "levels": [
    {
      "level": 1,
      "delay": "0s",
      "targets": [
        { "type": "on_call_schedule", "id": "payments-primary" }
      ],
      "notify_channels": ["in_app", "slack"]
    },
    {
      "level": 2,
      "delay": "5m",
      "targets": [
        { "type": "on_call_schedule", "id": "payments-secondary" }
      ],
      "notify_channels": ["in_app", "slack", "email"]
    },
    {
      "level": 3,
      "delay": "15m",
      "targets": [
        { "type": "user", "id": "team-lead-user-id" },
        { "type": "team", "id": "payments-team" }
      ],
      "notify_channels": ["in_app", "slack", "email", "voice"]
    }
  ]
}
```

## Target Types

| Target Type | Description | Use When |
|-------------|-------------|----------|
| `user` | A specific person by user ID | You want to page a named individual (e.g., a domain expert or team lead) |
| `team` | All members of a team | The whole team should be aware (e.g., at the final escalation level) |
| `on_call_schedule` | Whoever is currently on call per a schedule | The most common target — pages whoever the rotation says is responsible |

## Looping (Repeat)

Escalation policies **loop** through their levels. After the final level is reached without acknowledgement, escalation loops back to level 1 and repeats. The maximum number of loops is controlled by `repeat_count` (default: `3`). After `repeat_count` loops complete without acknowledgement, escalation gives up.

Set `repeat_count` to `0` for a single pass (no looping).

## SLA Integration

Escalation works hand-in-hand with [SLA tracking](/incident-management/sla):

- `sla_target_respond_at` — the response deadline. If this SLA is breached, escalation urgency may increase.
- `sla_target_resolve_at` — the resolution deadline.

SLA targets are derived from the incident's [priority](/incident-management/) (P1–P5) and the service's configured SLA policy. Higher-priority incidents get tighter deadlines and faster escalation.

## Creating an Escalation Policy

1. Go to **On-Call → Escalation Policies → Create Policy**
2. Name it (e.g., `payments-escalation`)
3. Set the **repeat count** (how many times to loop through all levels)
4. Add levels:
   - Set the **delay** for each level
   - Add **targets** (users, teams, or on-call schedules)
   - Choose **channels** for each level
5. Save
6. Assign the policy to one or more [services](/service-management/) or [teams](/on-call/)

## Best Practices

- **Start with the on-call schedule** at level 1 — page whoever the rotation says is responsible, not a specific person
- **Increase urgency per level** — level 1 might be in-app + Slack, level 2 adds email, level 3 adds voice
- **Don't make levels too deep** — 3–4 levels is usually enough. More levels means slower response
- **Use looping sparingly** — a high `repeat_count` with voice calls can wake people up repeatedly. 2–3 loops is typical
- **Always have a final safety net** — the last level should reach a team lead or manager who can manually intervene

## API Endpoints

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/escalation-policies` | Session | `escalation:read` | List escalation policies |
| `POST` | `/api/v1/escalation-policies` | Session | `escalation:write` | Create policy |
| `GET` | `/api/v1/escalation-policies/{id}` | Session | `escalation:read` | Get policy with levels and targets |
| `PATCH` | `/api/v1/escalation-policies/{id}` | Session | `escalation:write` | Update policy |
| `DELETE` | `/api/v1/escalation-policies/{id}` | Session | `escalation:write` | Delete policy |

## See Also

- [On-Call Schedules](/on-call/schedules) — define who's on call for each rotation
- [Teams](/on-call/) — group users and link policies
- [Notification Preferences](/on-call/notification-preferences) — per-user channel rules
- [SLA Tracking](/incident-management/sla) — response and resolution deadlines
