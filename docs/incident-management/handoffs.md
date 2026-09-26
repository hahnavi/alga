---
title: Shift Handoffs
description: How outgoing and incoming on-call hand off the pager with notes and acknowledgment.
---

# Shift Handoffs

> This page is about **on-call shift handoffs** — passing the pager from one shift to the next. It is not about swapping the incident commander during an incident (to rotate incident roles, end one role slot and assign a replacement on the incident itself).

When one on-call shift ends and another begins, Alga tracks the handover so context doesn't get lost.

## How It Works

```
Shift ending → outgoing leaves notes → handoff waiting → incoming acknowledges → done
```

1. The person ending their shift writes **outgoing notes**: open issues, things to watch, anything the next person needs to know.
2. The person starting their shift writes **incoming notes** and clicks **Acknowledge**.
3. Each step is logged so you can see who handed off to whom and when.

Only the outgoing person can edit outgoing notes, and only the incoming person can acknowledge.

## What You Click

- **Outgoing:** open the handoff, write your notes, save.
- **Incoming:** open your pending handoff, add your notes, click **Acknowledge**.

Pending handoffs for you are surfaced prominently so nothing slips.

## Pager Load

To check if the load is fair, open the pager metrics view. Pick a schedule and a date range to see per-shift stats: how many alerts fired on each shift, how many were acknowledged or missed, and average time to acknowledge.

You can also group by person to compare load across responders.

## Tips

- Always leave outgoing notes — short and actionable beats long and exhaustive.
- Acknowledge promptly so everyone knows the pager is covered.
- Use time-off overrides for planned absences instead of informal swaps.
- Check pager load regularly to spot overloaded rotations.

## See Also

- [On-Call Schedules](/on-call/schedules) — rotas, layers, and overrides
- [Escalation Policies](/on-call/escalation-policies) — who gets paged and when
- [ICS Roles](/incident-management/ics-roles) — incident commander and responders (different from shift handoffs)
