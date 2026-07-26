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
│  Webhook token auth → dedup → maintenance window    │
│  check → route → deliver                            │
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
┌──────────────────┐   ┌───────────────────────────┐
│   AI AGENT       │   │  INCIDENT                 │
│   (Hermes /      │   │  Created with ICS roles,  │
│    OpenClaw)     │   │  SLA, escalation, war     │
│                  │   │  room, coordination tasks  │
│  Receives via    │   └───────────────────────────┘
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
- **Labels** — key-value pairs (e.g. `alertname=HighCPU`, `namespace=production`, `service=api-gateway`) used for routing, correlation, triage rules, agent scope matching, and maintenance window suppression.
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

There are two kinds of investigations, each with its own lifecycle:

**Alert Investigation** (triggered by correlated alerts):

`pending → assigned → investigating → complete`

Additional states: `promoted` (escalated to an incident), `failed`, `cancelled`, `timed_out`, `paused`. Investigations can be `requeued` back to `pending` for reassignment.

**Incident Investigation** (triggered by an active incident):

`pending → assigned → investigating → complete`

Additional states: `cancelled`, `paused`, `coordinating` (agent is coordinating multi-agent tasks).

::: warning "assigned" not "delegated"
The scheduler *assigns* investigations to agents. You may see older references to "delegated" — the correct status is `assigned`.
:::

### Incidents

An **incident** is a declared event requiring coordinated response. Incidents can be:

- **Auto-promoted** from an alert investigation (the agent uses `alga_promote_to_incident`)
- **Manually created** by an operator
- **Auto-created** by routing rules or triage decisions

Incidents follow a formal lifecycle: `detected → triaging → active → mitigated → resolved → closed` (with `cancelled` as a terminal state).

Beyond the lifecycle, incidents carry several collaboration features:

- **Coordination tasks** — parent/child task trees dispatched to agents (claim/complete lifecycle: `pending → assigned → in_progress → complete`, or `failed`/`cancelled`)
- **ICS documents** — structured incident sections (`current_status`, `impact_assessment`, `root_cause`, `resolution`, `actions_taken`, `open_questions`, `resources`, `timeline_summary`) that are collaboratively edited by agents and operators
- **Status updates** — public incident status updates posted to notification channels
- **War rooms** — auto-created Google Meet spaces for real-time coordination (provisioned asynchronously by the ICS worker)

### Services

A **service** is a tracked component in your infrastructure (e.g. `api-gateway`, `payment-service`, `postgres-primary`). Services have:

- A **priority weight** used for status scoring
- **Dependencies** — a directed graph for cascade analysis (if service A depends on B, and B has an incident, A's status is affected)
- A linked **on-call schedule** and **escalation policy**

### Maintenance Windows

A **maintenance window** is a label-matched suppression window that silences alerts during planned maintenance. Each window has:

- A **name** and a time range (`start_time` to `end_time`)
- **Label matchers** — alerts whose labels match are suppressed for the duration of the window
- An **enabled** flag to toggle without deleting

::: tip Maintenance windows suppress, not delete
Alerts matching an active maintenance window are suppressed at ingestion — they are recorded but not routed, triaged, or dispatched. This keeps your alert history intact while avoiding noise during planned work.
:::

## How Components Relate

### Routing → Correlation → Triage → Investigation

These four stages process every alert, in order:

1. **Routing** — label-based rules determine *where* to deliver the alert (Slack, Mattermost, etc.) and whether to suppress it
2. **Correlation** — related alerts within the time window are grouped into one investigation
3. **Triage** — deterministic rules first (fast, free), then LLM (smart, costs tokens) decide what to do: `investigate`, `auto_resolve`, `suppress`, `escalate`, or `enrich_only`
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

- **Capabilities** (`investigate`, `communicate`, `command`) — what the agent is allowed to *do*
- **Scope** (`all` or `labels` via label selectors) — which investigations the agent is allowed to *receive*

The scheduler scores all eligible agents by specificity (label-matched > catch-all), then by load (least busy), then by health (success rate).

::: tip Capability meanings
- `investigate` — receive and work alert/incident investigations
- `communicate` — post messages to alert and incident threads
- `command` — coordinate incident command decisions, escalation, and multi-agent task dispatch
:::

### Threads → Real-Time Chat

Alerts and incidents have **owner-thread chat** — a conversation thread scoped to the alert or incident. Threads support:

- Real-time message delivery via SSE
- Typing indicators
- Cross-provider sync (messages posted in Alga appear in the linked Slack or Mattermost thread, and vice versa)

Thread owners include alert investigations, incident investigations, and incident coordination channels.

## Key Design Principles

### Deduplication at the Database Level

Alga uses partial unique indexes to enforce one-open-alert-per-fingerprint at the database level — not in application code. This means duplicate alerts are safe even under concurrent ingestion.

### Resolved Alerts Stay Resolved

Once an alert is resolved (by the monitoring system, an operator, or an agent), it is **never automatically reopened**. If the same condition fires again, a new alert is created. This prevents alert flapping.

### Fire-and-Forget Audit Logging

Every create, update, delete, command, and state transition produces an audit event. Audit logging is asynchronous — it never blocks the request that triggered it.

### Async Everything

Alert processing, notifications, investigations, escalation, SLA timers, and triage all run through RabbitMQ queues with tiered retry and dead-letter handling. The HTTP ingestion path returns immediately after persisting the alert — all downstream work is async.

The specific workers that consume these queues:

| Worker | Responsibility |
|--------|---------------|
| AlertWorker | Webhook ingestion, dedup, routing, delivery |
| InvestigateWorker | Dispatch investigations to agents, manage lifecycle |
| IncidentWorker | Incident state transitions, ICS role assignment |
| EscalationWorker | Execute escalation policy levels |
| SLAWorker | Track SLA timers, trigger breach escalation |
| NotificationDispatchWorker | Fan out notifications to configured channels |
| EmailWorker | Send transactional and notification emails |
| ICSWorker | Provision incident war rooms (Google Meet) |

Sweep workers run on timers to reconcile state:

| Sweep Worker | Responsibility |
|-------------|---------------|
| EscalationSweep | Detect stalled escalations and advance them |
| HeartbeatSweep | Detect missing heartbeats and fire stale alerts |
| StuckInvestigationEscalation | Escalate investigations stuck too long in active states |
| ActionItemSweep | Track overdue incident action items |
| Outbox | Retry and publish pending outbox messages |

### Fail-Closed Security

Alga refuses to start without encryption keys and secret pepper configured — in **every** environment, not just production. HSTS is always emitted on HTTPS. Tokens and secrets are stored as HMAC hashes or encrypted values, never plaintext.

## Where to Go Next

- **Set up your first alert source** — [First Steps Guide](/getting-started/first-steps)
- **Connect an AI agent** — [Hermes](/agents/hermes) or [OpenClaw](/agents/openclaw)
- **Configure routing** — [Routing](/core-features/routing)
- **Set up on-call** — [On-Call Schedules](/on-call/schedules)
- **Understand investigations** — [AI Investigation](/core-features/investigation)
- **Configure triage** — [Triage](/core-features/triage)
