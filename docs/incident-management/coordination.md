---
title: Incident Coordination
description: Chat with your team during an incident and keep stakeholders updated.
---

# Incident Coordination

Each incident has a chat stream for responders plus short public updates for stakeholders. If your team uses Slack, you can also get a dedicated channel per incident.

## Team Chat

Open the **Coordination** tab on the incident to talk with responders.

You can post three kinds of messages:

- **Chat** — normal discussion (the default).
- **Decision** — a key call you made, so it's easy to find later.
- **Action** — something you did or plan to do.

Everything else in the stream is written by Alga itself — status changes, assignments, and system notes appear automatically. You don't need to log those by hand.

## Stakeholder Updates

When people outside the response team ask "what's going on?", post a **Status update**. Each update is also added to the timeline.

Pick the phase that matches where you are:

| What you pick   | What it means                               |
| --------------- | ------------------------------------------- |
| Looking into it | We're checking what's wrong                 |
| Found it        | We know the cause                           |
| Contained       | We've stopped it spreading, fix is in place |
| Watching        | Fix is out, we're making sure it holds      |
| Fixed           | Confirmed healthy                           |

Tip: post early and often. A short "looking into it" is better than silence, and a communicator should own these during big incidents.

## Slack Channels

If your admin has turned on per-incident Slack channels, you'll see a **Create Slack channel** button on the incident. Click it to get a dedicated channel where chat messages and status updates appear. Replies in Slack sync back to the incident.

Ask your admin about channel settings (public or private, when channels are created, whether they archive on close) if the button is missing.

## See Also

- [Incident Lifecycle](/incident-management/lifecycle) — steps and buttons
- [ICS Roles](/incident-management/ics-roles) — who decides, updates, and fixes
- [Incident Overview](/incident-management/) — creating and working with incidents
