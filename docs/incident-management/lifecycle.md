---
title: Incident Lifecycle & States
description: What each incident status means and which buttons move you forward.
---

# Incident Lifecycle & States

Incidents move forward one step at a time so nothing gets skipped.

## The Steps

```
detected → triaging → active → mitigated → resolved → closed
```

Plus two side paths:

- **Cancelled:** for false alarms. You can cancel from detected, triaging, or active.
- **Reopen:** if the problem comes back, you can reopen it and it goes back to active.

## What Each Status Means

| Status    | What it means                                         | What you do                                     |
| --------- | ----------------------------------------------------- | ----------------------------------------------- |
| Detected  | Just arrived, waiting for someone to check it         | Click **Begin triage**                          |
| Triaging  | Someone is checking if it's real and how urgent it is | Click **Promote** to make it active             |
| Active    | Confirmed, response is underway                       | Work it, then click **Mitigate** when contained |
| Mitigated | Contained or fix is in place, you're watching it      | Click **Resolve** when you're sure it's fixed   |
| Resolved  | Fixed                                                 | Click **Close** to wrap up                      |
| Closed    | Wrapped up                                            | Done (you can still reopen if needed)           |
| Cancelled | Wasn't a real incident                                | Done                                            |

## You Can't Skip Steps

The buttons only work in order:

- **Mitigate** only works from active.
- **Resolve** only works from active or mitigated.
- **Close** only works from resolved.
- **Cancel** works from detected, triaging, or active.

If a button is greyed out, the incident isn't at the right step yet.

### What You Need Before Resolving

The **Resolve** button won't work until you've filled in:

- A short summary on the incident
- Impact (what was affected)
- Root cause (why it happened)
- Resolution (how you fixed it)

You'll see a message listing what's missing.

## If Two People Click at Once

If two people click a status button at the same time, one change wins. The other person sees a "changed concurrently" message. Just refresh the page to see the latest status and try again.

## What Happens Automatically

When the status changes, Alga:

- Adds a note to the timeline so everyone can see what happened
- Updates the linked service's health color
- Clearing a resolved incident also clears its linked firing alerts
- Stops pending pages when you acknowledge, resolve, close, or cancel

## See Also

- [Incident Overview](/incident-management/) — creating and working with incidents
- [ICS Roles](/incident-management/ics-roles) — who does what during response
- [SLA Tracking](/incident-management/sla) — response and resolution countdowns
- [Post-Mortems](/incident-management/post-mortems) — review after it's fixed
