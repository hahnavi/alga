---
title: Status Pages
description: Share service health with your team or customers in one place.
---

# Status Pages

A status page is one link where people can check "is it down?" — so your team can fix the problem instead of answering the same question in chat.

## Concepts

- **Status page** — a named page like "Production status," shared internally with your team or publicly with customers.
- **Component** — one line on the page, like "API," "Web dashboard," or "Database."
- **Overall status** — the worst status of any component. If one thing is red, the whole page shows red.

## What viewers see

Each component shows green, yellow, or red:

- **Green** — working normally
- **Yellow** — degraded or under maintenance, but still up
- **Red** — part or all of it is down

If an incident is affecting the page, viewers also see which incident is active.

For now, viewers need to log in to see status pages, even pages marked public.

## Creating a status page

1. Go to **Status Pages → Create Status Page**.
2. Give it a name and a short web address (slug), like `prod-status`.
3. Choose who it's for: **internal** (your team) or **public** (customers).
4. Add components like "API" and "Database," optionally linked to services you already track.

## Updating during an incident

When something breaks, open the status page, find the affected component, and change its color to yellow or red. When the incident is fixed, set it back to green.

## See also

- [Service Catalog](/service-management/) — link components to services
- [Incident Management](/incident-management/) — incidents that drive status updates
