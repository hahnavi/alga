---
title: Service Catalog
description: See service health at a glance — green, yellow, orange, red — and who owns what.
---

# Services

The service catalog shows the health of everything you run and who looks after it.

Lists show 50 services per page. Open a service to see its health, linked incidents, owners, and dependencies.

## Health Colors

Each service shows a color based on its active incidents and their urgency:

| Color  | What you see   | What it means                               |
| ------ | -------------- | ------------------------------------------- |
| Green  | Operational    | No active incidents                         |
| Yellow | Degraded       | Minor issues, some impact                   |
| Orange | Partially down | Significant outage — a single P1 lands here |
| Red    | Major outage   | Big outage — for example, two P1s at once   |

When all linked incidents are resolved, the service goes back to green automatically.

Services can also show **Maintenance** when work is planned.

## What You'll See on a Service

- **Name and description** — what the service is.
- **Owner team** — who looks after it.
- **Health** — current color, updated as incidents change.
- **Response goals** — optional custom respond/resolve times for incidents on this service.
- **Labels** — extra info tags. These are just stored notes for now; Alga doesn't auto-match alerts to services from them. Link incidents to services by choosing the service on the incident.
- **Dependencies** — which services this one relies on, and which rely on it.

> Note: connecting an escalation policy to a service is done through the API — there's no picker for it in the web app yet. Ask your admin or use the API.

## Dependencies

You can record that one service depends on another (for example, checkout depends on payments). This draws a map of what connects to what.

Each service's color reflects its own incidents — an outage on one service doesn't automatically recolor the services that depend on it. Link the incident to each affected service to show impact there.

## See Also

- [Incident Management](/incident-management/) — how incidents update service health
- [Escalation Policies](/on-call/escalation-policies) — who gets paged for a service's incidents
- [Teams](/on-call/) — who owns what
