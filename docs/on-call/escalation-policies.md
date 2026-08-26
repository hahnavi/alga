---
title: Escalation Policies
description: Multi-tier escalation policies with configurable delays, looping, per-level channel selection, and user or team targets (a team resolves to its on-call schedule).
---

# Escalation Policies

Escalation policies define **who gets notified, in what order, and how fast** when an incident isn't acknowledged. Alga's policies are multi-tier with configurable delays, looping, and per-level channel selection — so you can page level 1 by Slack, escalate to level 2 by SMS, and loop until someone responds.

## How Escalation Works

When an incident is created on a service, Alga loads that service's escalation policy and begins the escalation timer:

```
Incident created
      │
      ▼
 Level 1 pages immediately
      │
      │  Still unacknowledged after delay_minutes[level 1]?
      ▼
 Level 2 pages immediately
      │
      │  Still unacknowledged after delay_minutes[level 2]?
      ▼
 Level 3 pages immediately
      │
      │  Still unacknowledged after delay_minutes[level 3]?
      ▼
 Loop back to Level 1 (up to repeat_count times)
      │
      ▼
 Policy exhausted — escalation stops
```

Every level pages **immediately** when reached. `delay_minutes` is an **exit delay**: the quiet period _after_ level N pages during which nobody may acknowledge before Alga advances to level N+1. So `{level 1: 5}` does not mean "page level 1 after 5 minutes" — it means "if level 1 went unacknowledged for 5 minutes, page level 2."

**Acknowledgement stops escalation immediately.** The moment any responder acknowledges the incident — via the UI, Slack, or the API — all pending escalations are cancelled.

## Policy Structure

Each escalation policy has multiple **levels** triggered sequentially, stored as a JSON array on the policy. Each level specifies:

- **`level_number`** — the canonical ordering key (positive integer; levels are consumed in `level_number` order)
- **`delay_minutes`** — quiet period, in minutes, after this level pages before escalating to the next level (exit delay; see the ladder above). Sub-minute values are clamped to 1 minute.
- **`targets`** — who gets notified at this level (users or teams)
- **`notify_channels`** — which channels to use at this level (e.g., `["in_app", "email"]`, `["slack", "voice"]`)

### Example: Three-Tier Policy

A common pattern — page the primary on-call immediately, escalate to the secondary if nothing responds within 5 minutes, then pull in the team lead another 15 minutes after that:

```json
{
  "name": "payments-escalation",
  "repeat_count": 3,
  "levels": [
    {
      "level_number": 1,
      "delay_minutes": 5,
      "targets": [{ "target_type": "team", "target_team_id": "payments-team-id" }],
      "notify_channels": ["in_app", "slack"]
    },
    {
      "level_number": 2,
      "delay_minutes": 15,
      "targets": [{ "target_type": "team", "target_team_id": "payments-secondary-team-id" }],
      "notify_channels": ["in_app", "slack", "email"]
    },
    {
      "level_number": 3,
      "delay_minutes": 30,
      "targets": [{ "target_type": "user", "target_user_id": "team-lead-user-id" }],
      "notify_channels": ["in_app", "slack", "email", "voice"]
    }
  ]
}
```

With this policy: level 1 pages right away; if nobody acknowledges within 5 minutes, level 2 pages; if level 2 also goes unacknowledged for 15 minutes, level 3 pages; if level 3 stays unacknowledged for 30 minutes, the policy loops back to level 1 (up to `repeat_count` times).

## Target Types

| Target Type | Description                           | Use When                                                                                                           |
| ----------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `user`      | A specific person by `target_user_id` | You want to page a named individual (e.g., a domain expert or team lead)                                           |
| `team`      | A team by `target_team_id`            | The most common target — resolves to whoever is currently on call for the team's auto-provisioned on-call schedule |

A `team` target does **not** page every member of the team. It resolves through the team's on-call schedule, so the rotation (and any active override) decides who is paged. To page a named person directly, use a `user` target.

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
   - Add **targets** (users or teams)
   - Choose **channels** for each level
5. Save
6. Assign the policy to one or more [services](/service-management/)

## Best Practices

- **Start with a team target** at level 1 — it resolves to whoever the team's on-call rotation says is responsible, not a specific person
- **Increase urgency per level** — level 1 might be in-app + Slack, level 2 adds email, level 3 adds voice
- **Don't make levels too deep** — 3–4 levels is usually enough. More levels means slower response
- **Use looping sparingly** — a high `repeat_count` with voice calls can wake people up repeatedly. 2–3 loops is typical
- **Always have a final safety net** — the last level should reach a team lead or manager who can manually intervene

## API Endpoints

| Method   | Path                               | Auth    | Permission         | Description                        |
| -------- | ---------------------------------- | ------- | ------------------ | ---------------------------------- |
| `GET`    | `/api/v1/escalation-policies`      | Session | `escalation:read`  | List escalation policies           |
| `POST`   | `/api/v1/escalation-policies`      | Session | `escalation:write` | Create policy                      |
| `GET`    | `/api/v1/escalation-policies/{id}` | Session | `escalation:read`  | Get policy with levels and targets |
| `PATCH`  | `/api/v1/escalation-policies/{id}` | Session | `escalation:write` | Update policy                      |
| `DELETE` | `/api/v1/escalation-policies/{id}` | Session | `escalation:write` | Delete policy                      |

## See Also

- [On-Call Schedules](/on-call/schedules) — define who's on call for each rotation
- [Teams](/on-call/) — group users and link policies
- [Notification Preferences](/on-call/notification-preferences) — per-user channel rules
- [SLA Tracking](/incident-management/sla) — response and resolution deadlines
