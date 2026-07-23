---
title: Core Concepts
description: Understand how alerts, investigations, incidents, services, routing, triage, on-call, escalation, and agents relate in Alga's domain model.
---

# Core Concepts

Understanding how Alga's pieces fit together makes everything else easier to configure. This page explains the mental model: what happens when an alert fires, how it flows through the system, and how each component relates to the others.

## The Big Picture

```
                    ┌──────────┐
                    │  Alert   │  Webhook from Grafana, Prometheus, or any HTTP source
                    │  Source  │
                    └────┬─────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────┐
│                    INGESTION                         │
│  Webhook token auth → dedup → route → deliver       │
└──────────────────────┬──────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────┐
│                   CORRELATION                        │
│  Group related alerts by correlation key            │
│  within CORRELATION_WINDOW → one investigation      │
└──────────────────────┬──────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────┐
│                     TRIAGE                           │
│  Rules first (deterministic), then LLM              │
│  Decision: investigate / auto_resolve /             │
│           suppress / escalate / enrich_only         │
└──────────┬──────────────────────┬───────────────────┘
           │                      │
     investigate              escalate
           │                      │
           ▼                      ▼
┌──────────────────┐   ┌───────────────────┐
│   AI AGENT       │   │  INCIDENT         │
│   (Hermes /      │   │  Created with     │
│    OpenClaw)     │   │  ICS roles, SLA,  │
│                  │   │  escalation       │
│  Receives via    │   └───────────────────┘
│  SSE dispatch    │
│  Investigates,   │
│  resolves, or    │
│  promotes        │
└────────┬─────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│            NOTIFICATION                  │
│  In-app · Email · Slack · Mattermost    │
│  · Voice (Twilio/Telnyx)                │
└─────────────────────────────────────────┘
```

## Core Objects

### Alerts

An **alert** is a single firing or resolved event from a monitoring system. Each alert has:

- A **fingerprint** — a dedup key derived from its labels. Multiple firings of the same alert dedup to one record.
- An **alert number** — the human-readable unique ID (e.g. `#42`). This is what you'll reference in threads, URLs, and conversations.
- **Labels** — key-value pairs (e.g. `alertname=HighCPU`, `namespace=production`, `service=api-gateway`) used for routing, correlation, triage rules, and agent scope matching.
- **Annotations** — free-text metadata (summary, description, runbook URL).

::: tip The fingerprint is the dedup key, not the ID
The `alert_number` is the unique identifier you interact with. The fingerprint ensures that repeated firings of the same alert (same label set) don't create duplicates. One open alert per fingerprint is enforced by a partial unique index.
:::

### Investigations

An **investigation** is the AI analysis of one or more correlated alerts. When alerts arrive:

1. The **correlator** groups alerts sharing a correlation key within `CORRELATION_WINDOW` into a single investigation
2. The investigation enters `pending` status
3. The **scheduler** picks an online agent with matching capabilities and scope
4. The agent receives the investigation via SSE and begins analyzing

The investigation lifecycle: `pending → delegated → investigating → complete` (or `failed`, `timed_out`, `cancelled`, `paused`).

### Incidents

An **incident** is a declared event requiring coordinated response. Incidents can be:

- **Auto-promoted** from an alert investigation (the agent uses `alga_promote_to_incident`)
- **Manually created** by an operator
- **Auto-created** by routing rules or triage decisions

Incidents follow a formal lifecycle: `detected → triaging → active → mitigated → resolved → closed` (with `cancelled` as a terminal state).

### Services

A **service** is a tracked component in your infrastructure (e.g. `api-gateway`, `payment-service`, `postgres-primary`). Services have:

- A **priority weight** used for status scoring
- **Dependencies** — a directed graph for cascade analysis (if service A depends on B, and B has an incident, A's status is affected)
- A linked **on-call schedule** and **escalation policy**

## How Components Relate

### Routing → Correlation → Triage → Investigation

These four stages process every alert, in order:

1. **Routing** — label-based rules determine *where* to deliver the alert (Slack, Mattermost, etc.) and whether to suppress it
2. **Correlation** — related alerts within the time window are grouped into one investigation
3. **Triage** — deterministic rules first (fast, free), then LLM (smart, costs tokens) decide what to do: investigate, auto-resolve, suppress, escalate, or just enrich
4. **Investigation** — the scheduler dispatches the grouped alerts to an AI agent

### On-Call → Escalation → SLA

These three govern human response:

- **On-call schedules** define *who* is responsible right now (multi-layer rotations with overrides)
- **Escalation policies** define *who gets paged next* if the primary on-call doesn't respond (multi-level with timed delays)
- **SLA tracking** measures *how fast* the team responds and resolves, mapped by incident priority, and *triggers escalation* on breach

### Knowledge → Memory → Investigation

These three give agents the context they need:

- **Knowledge base** — operator-authored notes (runbooks, known issues, service owners, facts) with label selectors. You write these. Agents read them.
- **Agent memory** — LLM-extracted facts from past investigations, stored as vectors for semantic recall. The system learns these automatically.
- **Episodic context** — past investigations with the same correlation key, surfaced to the agent

All three are injected into the agent's dispatch prompt automatically.

### Agents → Capabilities → Scope

AI agents (Hermes, OpenClaw, or custom SDK agents) connect via SSE and receive investigation dispatches. Two gates control which investigations an agent receives:

- **Capabilities** (`investigate`, `communicate`, `triage`, `command`) — what the agent is allowed to *do*
- **Scope** (`all` or `labels`) — which investigations the agent is allowed to *receive*

The scheduler scores all eligible agents by specificity (label-matched > catch-all), then by load (least busy), then by health (success rate).

## Key Design Principles

### Deduplication at the Database Level

Alga uses partial unique indexes to enforce one-open-alert-per-fingerprint at the database level — not in application code. This means duplicate alerts are safe even under concurrent ingestion.

### Resolved Alerts Stay Resolved

Once an alert is resolved (by the monitoring system, an operator, or an agent), it is **never automatically reopened**. If the same condition fires again, a new alert is created. This prevents alert flapping.

### Fire-and-Forget Audit Logging

Every create, update, delete, command, and state transition produces an audit event. Audit logging is asynchronous — it never blocks the request that triggered it.

### Async Everything

Alert processing, notifications, investigations, escalation, SLA timers, and triage all run through RabbitMQ queues with tiered retry and dead-letter handling. The HTTP ingestion path returns immediately after persisting the alert — all downstream work is async.

### Fail-Closed Security

Alga refuses to start without encryption keys and secret pepper configured — in **every** environment, not just production. HSTS is always emitted on HTTPS. Tokens and secrets are stored as HMAC hashes or encrypted values, never plaintext.

## Where to Go Next

- **Set up your first alert source** — [First Steps Guide](/getting-started/first-steps)
- **Connect an AI agent** — [Hermes](/integrations/hermes) or [OpenClaw](/integrations/openclaw)
- **Configure routing** — [Routing](/core-features/routing)
- **Set up on-call** — [On-Call Schedules](/on-call/schedules)
- **Understand investigations** — [AI Investigation](/core-features/investigation)
- **Configure triage** — [Triage](/core-features/triage)
