---
title: Getting Started
description: Get Alga running in minutes with Docker Compose. Quick start, installation, first steps, and onboarding wizard.
---

# Getting Started

## Quick Start

```sh
git clone https://github.com/hahnavi/alga.git
cd alga
./setup.sh
docker compose up -d
```

This pulls ready-to-run images — no building or coding needed. Open `http://localhost:3000`. Since this is the first time, you'll see a setup wizard — just enter your email, full name, and a password to create your admin account. You'll only see this screen once.

When you log in for the first time, a friendly [Onboarding Wizard](/getting-started/onboarding) shows you around: what Alga does, where to connect your tools, and then you're off to your dashboard.

## Next Steps

| Guide                                                 | What You'll Learn                                   |
| ----------------------------------------------------- | --------------------------------------------------- |
| [Onboarding Wizard](/getting-started/onboarding)      | Guided first-run setup for new installations        |
| [Installation & Setup](/getting-started/installation) | Docker Compose, manual install, production setup    |
| [First Steps Guide](/getting-started/first-steps)     | Send test alerts, connect Grafana, explore features |

## Key Features to Explore

- **Incidents** — Full incident lifecycle with SLA tracking, escalation policies, and post-mortems
- **Services** — Service catalog with status tracking and dependency management
- **On-Call** — Multi-layer schedules with overrides and escalation policies
- **Teams** — Group users and link to escalation policies
- **AI Investigation** — Automated root cause analysis with the built-in Alga Agent, Hermes, or OpenClaw
- **Knowledge Base** — Shared notes for operators and agents with vector search
- **Routing** — Flexible alert routing to Slack, Mattermost, email, or voice

## Explore by Topic

- [Configuration](/configuration/environment-variables) — all environment variables
- [Alerts](/core-features/alerts) — alert lifecycle and ingestion
- [Incidents](/incident-management/) — incident management and SLA tracking
- [Services](/service-management/) — service catalog and dependencies
- [Teams & On-Call](/on-call/) — team management and on-call schedules
- [Routing](/core-features/routing) — route alerts to the right channels
- [Integrations](/integrations/) — connect Slack, Mattermost, or Twilio
- [AI Investigation](/core-features/investigation) — set up automated investigation

## Upgrading

```sh
git pull
docker compose pull
docker compose up -d
```

Alga updates the database on its own when it starts (this is turned on by default). To use a specific version, set `ALGA_VERSION=v1.2.3` in `.env`.

For manual migrations: `./alga db migrate`
