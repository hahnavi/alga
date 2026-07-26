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
    details: Receive alerts from Grafana, Prometheus, or any webhook source. Fingerprint-based deduplication with partial unique database indexes guarantees one open alert per issue — no duplicates, no noise.
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
    details: A pgvector-backed memory system extracts learnings from completed investigations. Agents recall past solutions via semantic search. Knowledge compounds over time — every incident makes the next one faster.
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
    details: Argon2id password hashing, AES-256-GCM envelope encryption, CSRF protection, ASVS Level 2 hardening, and constant-time comparisons. Your data stays on your infrastructure. MIT licensed.
    link: /configuration/security
    linkText: Security details →
---

::: warning Experimental Project
Alga is actively developed and **not yet production-ready**. Use at your own risk. Monitor AI token usage closely when autonomous investigation features are enabled. See the [README](https://github.com/hahnavi/alga) for current status.
:::

## What Can Alga Do?

Alga combines four traditionally separate tools into a single platform:

### 1. Alert Management
Ingest alerts from any webhook source (Grafana, Prometheus, custom scripts). Automatic fingerprint-based deduplication ensures you never get paged twice for the same issue. Alerts are correlated by deployment events and alertname within configurable time windows, then routed to the right destination via first-match rules.

### 2. AI Investigation
Every alert can trigger an autonomous investigation. Alga's scheduler atomically assigns work to an online agent (the native Alga Agent, Hermes, OpenClaw, or a custom SDK agent), which receives the full alert context, queries the knowledge base, searches its own memories of past incidents, and produces a structured root-cause analysis — all visible to operators in real time through investigation threads.

### 3. Incident Response
When an alert warrants it, the agent or an operator promotes it to an incident. Incidents follow a formal lifecycle (`detected → triaging → active → mitigated → resolved → closed`) with ICS command roles (Incident Commander, Communications Lead, Responder), SLA tracking, automated escalation, Google Meet war rooms, and structured post-mortems.

### 4. On-Call Management
Multi-layer schedules with follow-the-sun support, overrides, and structured handoffs ensure the right person is always reachable. Escalation policies loop through tiers until someone acknowledges. Pager-load metrics help balance the load across your team.

## Why Alga?

- **Open-source and self-hosted** — MIT licensed, runs on your infrastructure. No per-user pricing, no data leaving your control.
- **AI that actually investigates** — not just alert routing. Autonomous agents query your knowledge base, search past incidents, and produce structured findings before a human even looks at the alert.
- **Memory that compounds** — pgvector-backed episodic memory means every resolved investigation makes the next one faster. Agents get smarter the longer they run.
- **Incident response that's structured** — formal ICS roles, SLA tracking, and automated escalation borrowed from emergency management, not ad-hoc chat rooms.
- **Deduplication that works** — partial unique indexes at the database level enforce one-open-alert-per-fingerprint. No race conditions, no duplicates, ever.
- **Built for scale** — Valkey-backed agent presence and leader election, cross-replica SSE fan-out, and a three-level RabbitMQ retry topology with dead-lettering.

## Quick Start

```bash
git clone https://github.com/hahnavi/alga.git
cd alga
./setup.sh          # generates .env with random secrets
docker compose up -d
```

Open `http://localhost:3000` and complete the setup wizard to create the initial admin account (email, password, and full name). The wizard is only available the first time, before any admin exists.

→ Full setup in the [Installation Guide](/getting-started/installation) · New here? Start with [Core Concepts](/getting-started/concepts)

## Explore by Topic

| If you want to... | Read this |
|---|---|
| Understand how Alga works | [Core Concepts](/getting-started/concepts) |
| Get started fast | [Quick Start](/getting-started/) → [First Steps](/getting-started/first-steps) |
| Connect an AI agent | [Agents Overview](/agents/) · [Alga Agent](/agents/alga-agent) · [Hermes](/agents/hermes) · [OpenClaw](/agents/openclaw) |
| Configure alert routing | [Routing](/core-features/routing) · [Alerts](/core-features/alerts) |
| Set up on-call | [Schedules](/on-call/schedules) · [Escalation Policies](/on-call/escalation-policies) |
| Understand incidents | [Incident Management](/incident-management/) · [ICS Roles](/incident-management/ics-roles) |
| Deploy to production | [Deployment](/operations/deployment) · [Architecture](/operations/architecture) |
| Secure your instance | [Security & Auth](/configuration/security) · [Environment Variables](/configuration/environment-variables) |
| Build a custom integration | [Agent SDKs](/agents/agent-sdks) · [API Reference](/api-reference/) |
