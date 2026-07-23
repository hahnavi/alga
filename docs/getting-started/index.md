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

Open `http://localhost:3000`. When no users exist in the database, Alga automatically redirects to the setup wizard — create the initial admin account by entering an email, password, and full name. The setup wizard is only available the first time, before any admin exists, so complete it before logging in.

After the initial admin account is created, your first login triggers the [Onboarding Wizard](/getting-started/onboarding), which walks you through changing the admin password, connecting integrations, and configuring your first routing rules.

## Next Steps

| Guide | What You'll Learn |
|-------|-------------------|
| [Onboarding Wizard](/getting-started/onboarding) | Guided first-run setup for new installations |
| [Installation & Setup](/getting-started/installation) | Docker Compose, manual install, production setup |
| [First Steps Guide](/getting-started/first-steps) | Send test alerts, connect Grafana, explore features |

## Key Features to Explore

- **Incidents** — Full incident lifecycle with SLA tracking, escalation policies, and post-mortems
- **Services** — Service catalog with status tracking and dependency management
- **On-Call** — Multi-layer schedules with overrides and escalation policies
- **Teams** — Group users and link to escalation policies
- **AI Investigation** — Automated root cause analysis with Hermes/OpenClaw agents
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
docker compose up -d
```

Alga auto-migrates the database schema on startup when `POSTGRES_AUTO_MIGRATE=true` (enabled by default in Docker Compose).

For manual migrations: `./alga db migrate`
