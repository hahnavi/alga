---
title: Teams
description: Teams group users and link escalation policies and on-call schedules for incident response — the organizational backbone of on-call management.
---

# Teams

Teams group users together and serve as the organizational backbone for incident response in Alga. A team links members, [escalation policies](/on-call/escalation-policies), and [on-call schedules](/on-call/schedules) so that when an incident fires, the right people are notified through the right channels.

## How Teams Fit In

Teams connect several Alga concepts together:

```
  Team ──────── Members (users with admin/member roles)
    │
    ├── Escalation Policy ── multi-tier targets (users, teams, schedules)
    │
    ├── On-Call Schedules ── rotating coverage (who's on call right now)
    │
    └── Services ── team owns specific services in the catalog
```

When an alert becomes an incident on a service the team owns, Alga loads the team's escalation policy, checks who's on call from the team's schedules, and pages them — through the channels each person has configured in their [notification preferences](/on-call/notification-preferences).

## Team Structure

Each team has:

- **Members** — users assigned to the team, each with a role:
  - **Admin** — can add/remove members, manage schedules and escalation policies
  - **Member** — appears in the team's roster and can be targeted by escalation
- **Escalation Policy** — defines the multi-tier escalation chain for incidents assigned to this team
- **On-Call Schedules** — one or more rotating schedules that determine who is on call at any given time

## Creating a Team

1. Go to **On-Call → Teams → Create Team**
2. Give the team a **name** and optional description
3. **Add members** by searching for users and assigning roles
4. Link an **escalation policy** (or create one inline)
5. Link one or more **on-call schedules** (or create them inline)

A team doesn't need an escalation policy or schedule immediately — you can create the team first and attach policies and schedules later.

## Common Patterns

### Small Team (1–5 people)

A single team with one escalation policy that targets the on-call schedule, then loops back. Members rotate through a weekly schedule.

### Multi-Region Follow-the-Sun

Create separate teams per region (e.g., `sre-americas`, `sre-emea`, `sre-apac`), each with their own schedule. A top-level escalation policy pages the current region's on-call first, then escalates to the next region if unacknowledged.

### Service-Oriented Teams

Map teams to your service ownership. The `payments-team` owns payment services and has its own escalation policy with payment-domain experts. The `platform-team` owns infrastructure. Incidents route to the owning team based on the service's team assignment.

## API Endpoints

### Team Management

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/teams` | Session | `oncall:read` | List teams |
| `POST` | `/api/v1/teams` | Session | `oncall:write` | Create team |
| `GET` | `/api/v1/teams/{id}` | Session | `oncall:read` | Get team (includes members, escalation policy, schedules) |
| `PATCH` | `/api/v1/teams/{id}` | Session | `oncall:write` | Update team |
| `DELETE` | `/api/v1/teams/{id}` | Session | `oncall:write` | Delete team |

### Team Members

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/teams/{id}/members` | Session | `oncall:read` | List members with roles |
| `POST` | `/api/v1/teams/{id}/members` | Session | `oncall:write` | Add member (user_id, role) |
| `DELETE` | `/api/v1/teams/{id}/members/{userId}` | Session | `oncall:write` | Remove member |

## See Also

- [On-Call Schedules](/on-call/schedules) — rotating coverage and overrides
- [Escalation Policies](/on-call/escalation-policies) — multi-tier escalation chains
- [Notification Preferences](/on-call/notification-preferences) — per-user channel rules
- [Incident Management](/incident-management/) — how incidents trigger escalation
