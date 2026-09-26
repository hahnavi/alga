---
title: Onboarding Wizard
description: A short welcome guide that shows you around Alga the first time you log in.
---

# Onboarding Wizard

The first time you log in, Alga shows you a short welcome guide. It takes about a minute and simply points you to the right pages — it doesn't change any of your settings.

## How you get here

1. Open Alga. The very first time, you'll see a setup screen (`/setup`) — enter your email, full name, and password to create your admin account.
2. Log in. You'll land on the welcome guide (`/onboarding`).
3. Click through the screens. When you click **Go to dashboard**, the guide is done and you'll go to your dashboard.

Alga stays on the guide until you finish it — there's no skip button, but finishing takes just a few clicks.

## The 3 screens

### 1. Welcome

A quick hello explaining what Alga does: it collects your alerts, AI helpers investigate them, and you manage the response — all in one place.

### 2. Connect

Three tabs pointing you to where things live:

- **Webhook** — shows your alert receiving address (a URL you can copy). It reminds you to create a token on the **Incoming Webhooks** page (in the sidebar) and send it along with each alert. This is the same token used in the [First Steps Guide](/getting-started/first-steps) example.
- **Slack** — points you to the **Communication Channels** page, where you add your Slack connection so Alga can post alerts there.
- **Agent** — points you to the **Agents** page, where you create a token to connect an AI helper (the built-in Alga Agent, Hermes, or OpenClaw).

You don't have to do any of these now — you can come back to them later.

### 3. Done

A friendly "You're all set" screen. Click **Go to dashboard** to start using Alga. From there you can add channels, tokens, and helpers any time.

## Good to know

- The guide only appears until you finish it. After that, logging in takes you straight to the dashboard.
- It doesn't change your password, create tokens, or change settings — it just shows you where everything is.
- Behind the scenes, Alga simply remembers that you've finished the guide (a done/not-done flag).

## See Also

- [Installation & Setup](/getting-started/installation) — getting Alga running
- [First Steps Guide](/getting-started/first-steps) — send your first alert, connect Grafana
- [Security & Authentication](/configuration/security) — logins, roles, and sessions
