---
title: Escalation Policies
description: Who gets paged, in what order, and what happens if nobody responds.
---

# Escalation Policies

An escalation policy says **who gets paged, in what order, and how fast** when an incident isn't picked up.

## What Happens

```
Incident created → level 1 paged right away
  → still no response after a few minutes? → level 2 paged
    → still nothing? → level 3 paged
      → still nothing? → loops back through once or twice, then stops
```

- Each level pages **immediately** when its turn comes. The wait time is how long Alga gives that level to respond before moving on.
- Each round gets **louder**: early levels might be in-app and chat, later levels add email and phone calls.
- If someone **acknowledges**, all further paging stops right away.
- If nobody ever responds, the policy loops through a few times and then gives up.

Typically level 1 is the on-call rota and the last level is a team lead who can step in manually.

## Levels

Each level lists who gets paged and how:

- **Who:** a specific person, or a team (which means whoever's currently on call for that team's rota). Paging a team does not page everyone — just the person on call.
- **How:** which channels to use at that level (in-app, email, chat, voice).
- **How long to wait:** how many minutes to give them before moving to the next level.

Two or three loops through all levels is typical. More than that — especially with phone calls — gets noisy fast.

## Response Goals

Escalation works with [response countdowns](/incident-management/sla): if the respond-by time passes without anyone picking up and the incident has a policy, Alga pages the next level.

## Creating a Policy

1. Go to **On-Call → Escalation Policies → Create Policy**.
2. Give it a name (for example, `payments-escalation`).
3. Choose how many times it should loop.
4. Add levels: who to page, how to reach them, and how long to wait before moving on.
5. Save.

> Note: connecting a policy to a service is done through the API — there's no picker for it in the web app yet. Ask your admin or use the API.

## Tips

- Start with the on-call rota at level 1 — it reaches whoever's responsible right now.
- Get louder each level: quiet channels first, phone calls last.
- Keep it to 3–4 levels. More levels means slower response.
- Always end with a safety net — a lead or manager who can intervene by hand.

## See Also

- [On-Call Schedules](/on-call/schedules) — who's on call for each rota
- [Teams](/on-call/) — grouping people and rotas
- [Notification Preferences](/on-call/notification-preferences) — choose how you get paged
- [SLA Tracking](/incident-management/sla) — respond and resolve countdowns
