---
title: On-Call Schedules
description: Who's on call when — rotas, rotations, time off, and pager load.
---

# On-Call Schedules

Each team has **one rota**, created automatically when the team is created. The rota doesn't have its own name — it takes its name from the team.

## How Rotations Work

A rota is made of one or more **layers** — for example a Primary layer and a Secondary layer. Each layer says who rotates and how often:

- **How often:** hourly, daily, weekly, or monthly shifts.
- **Who:** the people in the rotation, in order.
- **When:** optional daily time windows and days of the week (for example, weekdays 9–5 in your timezone, or follow-the-sun coverage).
- **Priority:** if two layers cover the same moment, the higher-priority layer decides who's on call.

To change a rotation, open the team's rota and edit its layers.

## Time Off and Swaps

Use **overrides** for holidays, sick days, or coverage swaps. Pick the replacement person and the date range — while an override is active, that person is on call, no matter what the normal rotation says.

Overrides always win over layers, so use them instead of informal swaps. They're tracked, so everyone can see who covered whom.

## Who's On Call?

- **Who's on call now:** check the On-Call overview to see every rota's current person.
- **My shifts:** check your own view to see where you're currently on call and what's coming up in the next couple of weeks.
- **Calendar:** export a rota to your calendar app if you want reminders there.

## Shift Handoffs

When shifts change, the outgoing person leaves notes and the incoming person acknowledges. See [Shift handoffs](/incident-management/handoffs).

## Pager Load

To check if the load is fair, pick a schedule and a date range in the pager metrics view. You'll see per-shift stats: how many alerts fired on each shift, how many were acknowledged or missed, and average time to acknowledge — plus a summary across the range.

Group by person to compare load across responders instead of shift by shift.

## A Note on Phone Numbers

You can join a rota without a phone number. You only need one on file if you want to receive **voice call** pages — in-app, email, and chat pages work without it. You can also opt out of voice calls entirely in your notification preferences.

## See Also

- [Shift handoffs](/incident-management/handoffs) — passing the pager with notes
- [Escalation Policies](/on-call/escalation-policies) — who gets paged when nobody responds
- [Notification Preferences](/on-call/notification-preferences) — choose how you get paged
