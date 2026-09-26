---
title: SLA Tracking
description: Response and resolution countdowns based on urgency — what they mean and what stops them.
---

# SLA Tracking

Every incident shows two countdowns:

- **Respond by** — how fast someone needs to pick it up.
- **Resolve by** — how fast it needs to be fixed.

## Where Countdowns Come From

Countdowns come from the incident's **urgency (P1–P4)** — P1 is the fastest, P4 the slowest. They do not come from service settings.

When you create an incident you can accept the default times for its urgency or set your own custom times.

## What Stops the Clock

- **Acknowledging** stops further paging — Alga knows someone's got it.
- **Resolving, closing, or cancelling** stops any pending escalations.

## If You Miss the Respond Deadline

If nobody picks up in time and the incident has an escalation policy, Alga pages the next level (starting with level 1). You'll see a note on the timeline that the response goal was missed.

## Metrics You'll See

On the incidents metrics view you can see:

- **Time to acknowledge** — how long from creation to someone picking it up.
- **Time to mitigate** — how long to contain it.
- **Time to resolve** — how long to fix it.
- **Goal compliance** — what share of incidents met their respond and resolve goals.

Filter by date to spot trends.

## See Also

- [Incident Lifecycle](/incident-management/lifecycle) — steps and buttons
- [Escalation Policies](/on-call/escalation-policies) — who gets paged when nobody responds
- [Incident Overview](/incident-management/) — creating and working with incidents
