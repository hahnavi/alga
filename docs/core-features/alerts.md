---
title: Alerts
description: What alerts are, how to work through them, and what Alga does automatically.
---

# Alerts

An alert is Alga telling you "something needs attention." It usually arrives automatically from Grafana, or you can create one by hand from the Alerts page.

Each alert has its own number — like `#42`. That number is the ID you see in the list, in links, and when you search. Use it when you talk about an alert with your team.

## Lifecycle

Every alert moves through the same simple flow:

1. **Firing** → needs attention
2. **Acknowledged** → someone clicks Acknowledge to say "I'm looking at this"
3. **Resolved** → the problem is fixed

You will only ever see one open alert per issue. If the same problem fires again while it is still open, Alga adds it to the existing alert instead of creating a duplicate.

Resolved alerts stay resolved. They are never reopened by themselves — you can reopen one by hand if you need to.

## What you see on an alert

Open an alert to see its title, severity, when it started, where it came from, and a timeline of everything that happened to it.

The page updates live — you don't need to refresh to see new comments, status changes, or investigation updates.

You can from the alert page:

- **Acknowledge** — tell the team you're on it
- **Resolve** — mark the work as done
- **Reopen** — bring a resolved alert back if the problem returns
- **Investigate** — ask an AI helper to take a closer look
- **Comment** — discuss with your team in the thread under the alert

## Grouping related alerts

If several related alerts arrive within a few seconds of each other, Alga groups them together so you get one investigation instead of five.

By default Alga waits about 15 seconds to group related alerts; your admin can change this with CORRELATION_WINDOW (0 means send immediately).

## Linking alerts and incidents

You can link an alert to an incident from the link picker on the alert or incident page. Just start typing part of the name or `#number`, then pick from the suggestion list.

When you resolve an incident, Alga also resolves its linked alerts — except it skips any alert that is still linked to another active incident.

## When Alga stays quiet

- If an alert matches a silenced routing rule, Alga stores it but does not notify anyone or start an investigation.
- If an alert fires during a maintenance window that covers it, Alga stores it and stays quiet until the window ends.

## Adding alerts

- **Automatic:** create a token on the **Incoming Webhooks** page (click **Webhooks** in the sidebar), then point Grafana at Alga. New alerts just show up.
- **Manual:** click New Alert, give it a name and a message, and save.
