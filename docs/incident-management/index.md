---
title: Incident Management
description: How incidents move from detection to closure — what you see and click at each step.
---

# Incidents

When something breaks, Alga opens an incident so everyone knows what's happening, who's handling it, and how close you are to your response goals.

You work with incidents on the **Incidents page**. Open an incident to see its status, timeline, linked alerts, roles, messages, and documents in one place.

Lists show 50 items per page. Use search and filters to narrow by status, urgency, service, or date.

## Incident Lifecycle

Every incident moves step by step:

```
detected → triaging → active → mitigated → resolved → closed
```

- **Detected:** just arrived, nobody has picked it up yet.
- **Triaging:** someone is checking if it's real and how urgent it is.
- **Active:** confirmed, response is underway.
- **Mitigated:** the bleeding has stopped, you're watching to make sure it holds.
- **Resolved:** fixed and confirmed.
- **Closed:** wrapped up after resolving.
- **Cancelled:** false alarm (you can cancel from detected, triaging, or active).

You can't skip steps. For example, you can only mark an incident mitigated once it's active, and you can only resolve it from active or mitigated.

If two people click a status button at the same time, one change wins and the other person sees a "changed concurrently" message — just refresh and try again.

See [Lifecycle & States](/incident-management/lifecycle) for what each button does.

## Key Ideas

- **Start small:** new incidents arrive as detected. Click **Begin triage** to start checking, then **Promote** to make it active.
- **Response goals:** each incident has countdowns based on its urgency (P1 fastest, P4 slowest). You can set custom times when you create it. See [SLA Tracking](/incident-management/sla).
- **Acknowledge stops paging:** clicking **Acknowledge** tells Alga "we've got it" and stops further pages.
- **Who's who:** the commander decides, the communicator posts updates, responders fix things. Staff or AI helpers can fill these spots. See [ICS Roles](/incident-management/ics-roles).
- **Talk it through:** use the coordination chat and stakeholder updates. See [Coordination](/incident-management/coordination).
- **Shared notes:** each incident has a shared document (current status, impact, root cause, resolution, and more). You'll need to fill in impact, root cause, and resolution before you can resolve.
- **Linked alerts:** alerts linked to an incident are shown on the incident. Resolving the incident clears its linked firing alerts.
- **Shift handoffs:** when on-call shifts change, the outgoing person leaves notes and the incoming person acknowledges. See [Shift handoffs](/incident-management/handoffs).

## Creating Incidents

1. Go to **Incidents → New incident**.
2. Give it a title, description, urgency (P1–P4), and service if you know it.
3. Save. It appears as detected and starts its response countdowns.

Incidents can also be created from an alert you're triaging.

## See Also

- [Lifecycle & States](/incident-management/lifecycle) — what each status means and which buttons move you forward
- [ICS Roles](/incident-management/ics-roles) — commander, communicator, responders
- [Coordination](/incident-management/coordination) — chat and stakeholder updates
- [Shift handoffs](/incident-management/handoffs) — on-call shift notes and acknowledgment
- [SLA Tracking](/incident-management/sla) — response and resolution countdowns
- [Post-Mortems](/incident-management/post-mortems) — blameless review after it's fixed
