---
title: Routing
description: How Alga decides who should see each alert.
---

# Routing

Routing decides where each alert goes — which channel and team sees it.

You manage this under **Routes**. You will see a list of rules, checked from top to bottom.

## How it works

When an alert arrives:

1. Alga checks your rules in order. The first matching rule wins.
2. If no rule matches, the alert goes to your default channel.

That's it. Order matters — put specific rules at the top and general ones at the bottom.

## Rules

Each rule is simple:

- **Name** — what you call it, like "Production database"
- **When** — which alerts it applies to, like "namespace is production"
- **Where** — which Slack or Mattermost channel to send it to

You can match on things like the alert name, severity, or team labels. A rule can require all conditions to match, or just any one of them.

## Staying quiet on purpose

Sometimes you want Alga to ignore certain alerts:

- **Silenced rule = mute forever.** Create a rule, mark it silenced, and matching alerts are stored but never sent anywhere. Good for known noise you never want to see.
- **Maintenance window = mute during planned work.** Tell Alga a start and end time plus which alerts to ignore, and it stays quiet until the work is done. Good for migrations and deploys.

In both cases you can still find the alert in the list later — it just doesn't notify anyone or start an investigation.

## If nothing matches

Alerts that don't match any rule go to your default channel so nothing gets lost.
