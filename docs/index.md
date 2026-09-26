---
layout: home

hero:
  name: "Alga"
  text: "AI-Powered Incident Management"
  tagline: The open-source, self-hosted platform that ingests alerts, investigates them autonomously, orchestrates incident response, and routes to the right responder — so your team spends less time triaging noise and more time resolving what matters.
  image:
    src: /logo.svg
    alt: Alga logo
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started/
    - theme: alt
      text: Core Concepts
      link: /getting-started/concepts
    - theme: alt
      text: View on GitHub
      link: https://github.com/hahnavi/alga

features:
  - icon:
      src: /icons/alerts.svg
      alt: ""
    title: Alert Ingestion & Dedup
    details: Receive alerts from Grafana, Prometheus, or any webhook source. Smart grouping makes sure you get one open alert per problem — no duplicates, no noise.
    link: /core-features/alerts
    linkText: Learn about alerts →
  - icon:
      src: /icons/investigation.svg
      alt: ""
    title: Autonomous AI Investigation
    details: Built-in SRE agents — the native Alga Agent, Hermes, or OpenClaw — investigate every alert, querying knowledge, correlating signals, and producing structured root-cause analyses in parallel with human oversight.
    link: /core-features/investigation
    linkText: How investigation works →
  - icon:
      src: /icons/incident.svg
      alt: ""
    title: Incident Management with ICS
    details: Full incident lifecycle with formal Incident Command System roles, SLA tracking, automated escalation, Google Meet war rooms, and structured post-mortems with action items.
    link: /incident-management/
    linkText: Explore incidents →
  - icon:
      src: /icons/oncall.svg
      alt: ""
    title: On-Call & Escalation
    details: Multi-layer schedules with follow-the-sun support, overrides, structured handoffs with acknowledgment, and policy-driven multi-tier escalation that loops until someone responds.
    link: /on-call/schedules
    linkText: Set up schedules →
  - icon:
      src: /icons/memory.svg
      alt: ""
    title: Agent Memory & Knowledge
    details: A memory system that learns from finished investigations. Helpers remember past fixes and find them when similar problems come up. Knowledge builds over time — every incident makes the next one faster.
    link: /agents/memory
    linkText: How memory works →
  - icon:
      src: /icons/triage.svg
      alt: ""
    title: Triage & Noise Suppression
    details: Rule-based and LLM-powered triage classifies, prioritizes, and suppresses noise before it reaches a human. Agents provide feedback that improves accuracy over time.
    link: /core-features/triage
    linkText: Understand triage →
  - icon:
      src: /icons/playbooks.svg
      alt: ""
    title: Playbooks & Routing
    details: Label-selector-matched playbooks inject step-by-step response procedures into every investigation. Rule-based routing directs alerts to the right channel, team, or agent.
    link: /core-features/playbooks
    linkText: Create playbooks →
  - icon:
      src: /icons/security.svg
      alt: ""
    title: Secure & Self-Hosted
    details: Strong password protection, encrypted secrets, and hardened logins. Your data stays on your computers. MIT licensed.
    link: /configuration/security
    linkText: Security details →
---

::: warning Experimental Project
Alga is actively developed and **not yet production-ready**. Use at your own risk. Monitor AI token usage closely when autonomous investigation features are enabled. See the [README](https://github.com/hahnavi/alga) for current status.
:::

## What Can Alga Do?

Alga combines four traditionally separate tools into a single platform:

### 1. Alert Management

Ingest alerts from any webhook source (Grafana, Prometheus, custom scripts). Automatic grouping makes sure you never get paged twice for the same issue. Related alerts are bundled together, then sent to the right place by your rules.

### 2. AI Investigation

Every alert can trigger an investigation by an AI helper. Alga hands the work to an available helper (the built-in Alga Agent, Hermes, OpenClaw, or a custom helper), which gets the full alert details, reads your team's notes, remembers similar past incidents, and writes up what it found — all visible to you in real time.

### 3. Incident Response

When an alert warrants it, the agent or an operator promotes it to an incident. Incidents follow a formal lifecycle (`detected → triaging → active → mitigated → resolved → closed`) with ICS command roles (Incident Commander, Communications Lead, Responder), SLA tracking, automated escalation, Google Meet war rooms, and structured post-mortems.

### 4. On-Call Management

Multi-layer schedules with follow-the-sun support, overrides, and structured handoffs ensure the right person is always reachable. Escalation policies loop through tiers until someone acknowledges. Pager-load metrics help balance the load across your team.

## Why Alga?

- **Open-source and self-hosted** — MIT licensed, runs on your infrastructure. No per-user pricing, no data leaving your control.
- **AI that actually investigates** — not just alert routing. Autonomous agents query your knowledge base, search past incidents, and produce structured findings before a human even looks at the alert.
- **Memory that builds up** — every finished investigation teaches Alga something. Helpers get smarter the longer they run.
- **Response that's organized** — clear roles, response-time goals, and automatic escalation, not ad-hoc chat rooms.
- **No duplicates** — Alga keeps one open alert per problem, even when many copies arrive at once. No repeats, ever.
- **Ready to grow** — background workers keep things moving reliably as you add more alerts and helpers.

## Quick Start

```bash
git clone https://github.com/hahnavi/alga.git
cd alga
./setup.sh          # creates secret passwords automatically, nothing to edit
docker compose up -d
```

Open `http://localhost:3000` and fill in the setup screen to create your admin account (email, full name, and password). You'll only see this screen the first time.

→ Full setup in the [Installation Guide](/getting-started/installation) · New here? Start with [Core Concepts](/getting-started/concepts)

## Explore by Topic

| If you want to...          | Read this                                                                                                                |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Understand how Alga works  | [Core Concepts](/getting-started/concepts)                                                                               |
| Get started fast           | [Quick Start](/getting-started/) → [First Steps](/getting-started/first-steps)                                           |
| Connect an AI agent        | [Agents Overview](/agents/) · [Alga Agent](/agents/alga-agent) · [Hermes](/agents/hermes) · [OpenClaw](/agents/openclaw) |
| Configure alert routing    | [Routing](/core-features/routing) · [Alerts](/core-features/alerts)                                                      |
| Set up on-call             | [Schedules](/on-call/schedules) · [Escalation Policies](/on-call/escalation-policies)                                    |
| Understand incidents       | [Incident Management](/incident-management/) · [ICS Roles](/incident-management/ics-roles)                               |
| Deploy to production       | [Deployment](/operations/deployment) · [Architecture](/operations/architecture)                                          |
| Secure your instance       | [Security & Auth](/configuration/security) · [Environment Variables](/configuration/environment-variables)               |
| Build a custom integration | [Agent SDKs](/agents/agent-sdks) · [API Reference](/api-reference/)                                                      |
