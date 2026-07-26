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
    ├── On-Call Schedule ── one auto-provisioned schedule per team
    │       (rotating coverage: who's on call right now)
    │
    └── Services ── team owns specific services in the catalog
```

Escalation policies are configured separately and **target** teams (or individual users) as escalation targets — a team is not directly "linked" to a policy. When an alert becomes an incident on a service, Alga loads the service's [escalation policy](/on-call/escalation-policies), and any level that targets a team resolves to whoever is currently on call for that team's schedule. The resolved responder is then paged through the channels they configured in their [notification preferences](/on-call/notification-preferences).

## Team Structure

A team has a unique **name** and an optional **description**, plus:

- **Members** — users assigned to the team (`user_id` + `role`), each with a role:
  - **Admin** — can add/remove members, manage the schedule and members
  - **Member** — appears in the team's roster and can be targeted by escalation
- **On-Call Schedule** — exactly one schedule, auto-provisioned when the team is created. Its display name is derived from the team, and its rotating coverage is defined by [layers](/on-call/schedules)

## Creating a Team

1. Go to **On-Call → Teams → Create Team**
2. Give the team a **name** (unique) and optional description
3. **Add members** by searching for users and assigning roles

Creating a team automatically provisions its on-call schedule (one per team). You then configure that schedule's rotation layers separately. Escalation policies are created independently and reference the team (or its members) as targets.

## Common Patterns

### Small Team (1–5 people)

A single team with one escalation policy that targets the on-call schedule, then loops back. Members rotate through a weekly schedule.

### Multi-Region Follow-the-Sun

Create separate teams per region (e.g., `sre-americas`, `sre-emea`, `sre-apac`), each with their own schedule. A top-level escalation policy pages the current region's on-call first, then escalates to the next region if unacknowledged.

### Service-Oriented Teams

Map teams to your service ownership. The `payments-team` owns payment services and has its own escalation policy with payment-domain experts. The `platform-team` owns infrastructure. Incidents route to the owning team based on the service's team assignment.

## API Endpoints

### Team Management

| Method   | Path                 | Auth    | Permission     | Description |
| -------- | -------------------- | ------- | -------------- | ----------- |
| `GET`    | `/api/v1/teams`      | Session | `oncall:read`  | List teams  |
| `POST`   | `/api/v1/teams`      | Session | `oncall:write` | Create team |
| `GET`    | `/api/v1/teams/{id}` | Session | `oncall:read`  | Get team    |
| `PATCH`  | `/api/v1/teams/{id}` | Session | `oncall:write` | Update team |
| `DELETE` | `/api/v1/teams/{id}` | Session | `oncall:write` | Delete team |

### Team Members

| Method   | Path                                  | Auth    | Permission     | Description                |
| -------- | ------------------------------------- | ------- | -------------- | -------------------------- |
| `GET`    | `/api/v1/teams/{id}/members`          | Session | `oncall:read`  | List members with roles    |
| `POST`   | `/api/v1/teams/{id}/members`          | Session | `oncall:write` | Add member (user_id, role) |
| `PATCH`  | `/api/v1/teams/{id}/members/{userId}` | Session | `oncall:write` | Update member role         |
| `DELETE` | `/api/v1/teams/{id}/members/{userId}` | Session | `oncall:write` | Remove member              |

## See Also

- [On-Call Schedules](/on-call/schedules) — rotating coverage and overrides
- [Escalation Policies](/on-call/escalation-policies) — multi-tier escalation chains
- [Notification Preferences](/on-call/notification-preferences) — per-user channel rules
- [Incident Management](/incident-management/) — how incidents trigger escalation
