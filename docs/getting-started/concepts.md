---
title: Core Concepts
description: Understand how alerts, investigations, incidents, services, routing, triage, on-call, escalation, and agents relate in Alga's domain model.
---

# Core Concepts

Understanding how Alga's pieces fit together makes everything else easier to configure. This page explains the mental model: what happens when an alert fires, how it flows through the system, and how each component relates to the others.

## The Big Picture

```
                     ┌──────────┐
                     │  Alert   │  Message from Grafana, Prometheus, or any tool
                     │  Source  │
                     └────┬─────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│                    RECEIVING                        │
│  Check password → remove duplicates → hold back     │
│  during maintenance → send on                       │
└──────────────────────┬──────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────┐
│                   GROUPING                          │
│  Group related alerts together → one investigation  │
└──────────────────────┬──────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────┐
│                     DECIDING                        │
│  Simple rules first, then AI judgment               │
│  Decision: investigate / auto_resolve /             │
│           suppress / escalate / enrich_only         │
└──────────┬──────────────────────┬───────────────────┘
           │                      │
     investigate              escalate
           │                      │
           ▼                      ▼
┌──────────────────┐   ┌───────────────────────────┐
│   AI HELPER      │   │  INCIDENT                 │
│   (built-in      │   │  Managed with clear roles,│
│    Alga Agent,   │   │  response-time goals,     │
│    Hermes, or    │   │  escalation, and a shared │
│    OpenClaw)     │   │  chat room                │
│                  │   └───────────────────────────┘
│  Receives the    │
│  investigation,  │
│  looks into it,  │
│  resolves it or  │
│  raises it       │
└────────┬─────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│            NOTIFYING                     │
│  In Alga · Email · Slack · Mattermost   │
│  · Phone call                           │
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
The `alert_number` is the unique identifier you interact with. The fingerprint makes sure that repeated firings of the same alert (same labels) don't create duplicates — you get one open alert per problem.
:::

### Investigations

An **investigation** is the AI analysis of one or more correlated alerts. When alerts arrive:

1. Alga groups alerts about the same thing into a single investigation
2. The investigation starts out as `pending`
3. Alga picks an available helper whose skills match and hands it the investigation
4. The helper starts analyzing and reports back

There are two kinds of investigations, each with its own lifecycle:

**Alert Investigation** (triggered by correlated alerts):

`pending → assigned → investigating → complete`

Additional states: `promoted` (escalated to an incident), `failed`, `cancelled`, `timed_out`, `paused`. Investigations can be `requeued` back to `pending` for reassignment.

**Incident Investigation** (triggered by an active incident):

`pending → assigned → investigating → complete`

Additional states: `cancelled`, `paused`, `coordinating` (commander-owned orchestrator coordinating child investigations; excluded from normal scheduling).

::: warning "assigned" not "delegated"
The scheduler _assigns_ investigations to agents. You may see older references to "delegated" — the correct status is `assigned`.
:::

### Incidents

An **incident** is a declared event requiring coordinated response. Incidents can be:

- **Auto-promoted** from an alert investigation (the agent uses `alga_promote_to_incident`)
- **Manually created** by an operator
- **Auto-created** by routing rules or triage decisions

Incidents move through stages: `detected → triaging → active → mitigated → resolved → closed` (with `cancelled` if the incident turns out to be a false alarm, and `reopen` if a finished incident flares up again). You can also jump ahead when it makes sense — for example, confirming an incident moves it straight to `active` (see [Incident Lifecycle](/incident-management/lifecycle) for the full picture).

Beyond the lifecycle, incidents carry several collaboration features:

- **Coordination messages** — the per-incident thread where operators and agents collaborate via @mentions, agent replies, and structured handoffs
- **ICS documents** — structured incident sections (`current_status`, `impact_assessment`, `root_cause`, `resolution`, `actions_taken`, `open_questions`, `resources`, `timeline_summary`) that are collaboratively edited by agents and operators
- **Status updates** — public incident status updates posted to notification channels
- **War rooms** — auto-created Google Meet spaces for real-time coordination

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

1. **Routing** — rules based on alert labels decide _where_ to send the alert (Slack, Mattermost, etc.) and whether to hold it back
2. **Correlation** — related alerts that arrive close together are grouped into one investigation
3. **Triage** — simple rules run first (fast and free), then AI judgment (smarter, uses a little AI budget) decides what to do: `investigate`, `auto_resolve`, `suppress`, `escalate`, or `enrich_only`
4. **Investigation** — the grouped alerts are handed to an AI helper (the built-in Alga Agent, Hermes, or OpenClaw)

### On-Call → Escalation → SLA

These three govern human response:

- **On-call schedules** define _who_ is responsible right now (multi-layer rotations with overrides)
- **Escalation policies** define _who gets paged next_ if the primary on-call doesn't respond (multi-level with timed delays)
- **SLA tracking** measures _how fast_ the team responds and resolves, mapped by incident priority, and _triggers escalation_ on breach

### Knowledge → Memory → Investigation

These three give agents the context they need:

- **Knowledge base** — notes you write (how-to guides, known issues, who owns what). Helpers read them when investigating.
- **Agent memory** — lessons Alga learns on its own from past investigations. The system remembers these automatically.
- **Past cases** — earlier investigations about the same problem, shown to the helper for context

All three are handed to the helper automatically when it starts work.

### Agents → Skills → Scope

AI helpers (the built-in Alga Agent, Hermes, OpenClaw, or a custom helper) stay connected and receive investigations. Two settings control which work a helper gets:

- **Skills** (`investigate`, `communicate`, `command`) — what the helper is allowed to _do_
- **Scope** (`all` or picked by matching labels) — which investigations the helper is allowed to _receive_

Alga prefers helpers that match closely, then ones with the least on their plate.

::: tip Capability meanings

- `investigate` — receive and work alert/incident investigations
- `communicate` — post messages to alert and incident threads
- `command` — coordinate incident command decisions, escalation, and coordination messages
  :::

### Threads → Real-Time Chat

Alerts and incidents have their own chat thread — a conversation just about that alert or incident. Threads support:

- Messages appearing instantly
- Typing indicators
- Two-way sync (messages you write in Alga show up in the linked Slack or Mattermost thread, and vice versa)

Thread owners include alert investigations, incident investigations, and incident coordination channels.

## Key Design Principles

### No duplicate alerts

Alga guarantees one open alert per problem at the database level — not just in the app. Duplicate alerts are safe even if many arrive at the same moment.

### Resolved alerts stay resolved

Once an alert is resolved (by your monitoring, by you, or by a helper), it is **never automatically reopened**. If the same problem happens again, you get a new alert. This stops alerts from flickering open and closed.

The one exception is **manual reopen**: you can deliberately reopen the latest resolved alert (Alga records who did it). Automatic sources never reopen resolved alerts.

### Everything important is recorded

Every change — creating, updating, deleting, and every status change — is written to an audit log. This happens in the background, so it never slows you down.

### Work happens in the background

Alert processing, notifications, investigations, escalation, and reminders all run as background jobs with retries. The page that receives alerts saves them and returns right away — everything after that happens behind the scenes.

### Safe by default

Alga refuses to start without its secret passwords — in **every** setup, not just production. Passwords and tokens are stored scrambled or locked up, never as readable text.

## Where to Go Next

- **Set up your first alert source** — [First Steps Guide](/getting-started/first-steps)
- **Connect an AI agent** — [Alga Agent](/agents/alga-agent), [Hermes](/agents/hermes), or [OpenClaw](/agents/openclaw)
- **Configure routing** — [Routing](/core-features/routing)
- **Set up on-call** — [On-Call Schedules](/on-call/schedules)
- **Understand investigations** — [AI Investigation](/core-features/investigation)
- **Configure triage** — [Triage](/core-features/triage)
