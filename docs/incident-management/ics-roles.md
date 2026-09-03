---
title: ICS Incident Command System
description: Who does what during an incident — commander, communicator, and responders.
---

# ICS Incident Command System

When pressure is high, it helps to know who's in charge and who does what. Alga uses three simple roles for every incident.

## The Three Roles

| Role             | What they do                         |
| ---------------- | ------------------------------------ |
| **Commander**    | Decides direction, makes final calls |
| **Communicator** | Posts updates for stakeholders       |
| **Responders**   | Do the hands-on fixing               |

Staff members or AI helpers can fill any of these spots. You'll see who's in each role at the top of the incident page.

## Why This Helps

1. **Who's in charge?** — the commander owns decisions.
2. **Who does what?** — deciding, updating, and fixing are split so nothing falls through.
3. **How do we rotate?** — when someone needs a break, you hand their role to someone else.

## Assigning Roles

On the incident page, open the **Roles** section and click **Assign**. Pick the role and the person (or AI helper) taking it.

Alga may suggest the person currently on call as commander when the incident first comes in. If nobody is on call, the spot stays empty for you to fill.

## Rotating People

To hand a role to someone else:

1. End the current person's slot.
2. Assign a replacement.

This keeps a clean history of who held each role and when. Keep one active commander at a time, and bring in a communicator early — stakeholders want updates fast.

> Note: changing the detailed scope text on a role assignment is only available through the API, for user roles. In the web app you assign and end roles.

## Shared Incident Notes

Each incident has shared notes with sections like current status, impact, actions taken, open questions, root cause, and resolution.

You'll need impact, root cause, and resolution filled in (plus a summary) before you can resolve — the Resolve button will tell you what's missing.

## War-Room Calls

If your team uses Google Meet war rooms, you'll see a **Meet link** on active incidents to join the live call. Ask your admin to turn this on if you don't see it.

## Tips

- Keep a single active commander.
- End roles you're no longer using so the role list stays clean.
- Assign a communicator early, don't wait until the end.
- Let AI helpers take responder or communicator spots for routine investigating and updates.

## See Also

- [Lifecycle & States](/incident-management/lifecycle) — incident steps and buttons
- [Incident Coordination](/incident-management/coordination) — chat and status updates
- [Shift handoffs](/incident-management/handoffs) — on-call shift notes (different from incident roles)
