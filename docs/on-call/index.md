---
title: Teams
description: How teams, rotas, and escalation fit together — what you see and click.
---

# Teams

Teams group people together so the right person gets paged when something breaks.

## How It Fits Together

```
Team (people with lead / member labels)
  ├── Rota — one per team, created automatically (who's on call right now)
  └── Services — services the team looks after
```

When an incident fires on a service, Alga checks that service's escalation policy. If a level points at a team, whoever is currently on call for that team's rota gets paged, through the channels they chose in their notification preferences.

Each team gets **one rota automatically** when the team is created — you don't create it yourself, you just set up its rotation layers. Lead and member are descriptive labels on the roster, not different permission levels.

> Note: connecting an escalation policy to a service is done through the API — there's no picker for it in the web app yet. Ask your admin or use the API if you need to change which policy a service uses.

## Creating a Team

1. Go to **On-Call → Teams → Create Team**.
2. Give the team a name and optional description.
3. Add members by searching for people. Mark someone as lead if they're the team's point person.

Creating the team also creates its rota. Next, set up the rota's layers (who rotates and how often).

## Common Setups

### Small Team (1–5 people)

One team, one rota with a weekly rotation. The escalation policy pages whoever's on call first, then loops back if nobody responds.

### Follow-the-Sun

Create one team per region (for example, Americas, EMEA, APAC), each with its own rota. The escalation policy pages the current region first, then the next region if nobody picks up.

### Service-Oriented

Match teams to what they own — payments team owns payment services, platform team owns infrastructure. Incidents go to the owning team based on the service.

## See Also

- [On-Call Schedules](/on-call/schedules) — rotas, layers, and time off
- [Escalation Policies](/on-call/escalation-policies) — who gets paged and when
- [Notification Preferences](/on-call/notification-preferences) — choose how you get paged
- [Incident Management](/incident-management/) — how incidents trigger paging
