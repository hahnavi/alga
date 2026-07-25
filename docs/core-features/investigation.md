---
title: AI Investigation
description: How Alga's scheduler dispatches alerts to autonomous SRE agents (Hermes or OpenClaw) for root-cause analysis, the investigation lifecycle, and agent configuration.
---

# AI Investigation

Alga's AI investigation feature automatically analyzes alerts and produces root cause reports using autonomous AI agents. When alerts fire, Alga groups them, triages them, and dispatches them to a connected agent — which investigates, reports findings, and takes lifecycle actions like resolving the alert or promoting it to an incident.

## How It Works

Every alert flows through a pipeline:

```
Alert → Correlator → Triage → Scheduler → Agent (SSE dispatch)
                                              │
                                    ┌─────────┴──────────┐
                                    │                    │
                              Investigates           Promotes to Incident
                              Resolves               Coordinates multi-agent
                              Escalates              response
```

1. **Correlator** groups related alerts sharing a correlation key within `CORRELATION_WINDOW`
2. **Triage** evaluates the group — rules first (deterministic), then LLM — and decides what to do (investigate, auto-resolve, suppress, escalate)
3. **Scheduler** picks the best online agent by capability match, label scope, load, and health
4. **Agent** receives the investigation via SSE, reasons about root cause using its tools, and reports back
5. **Results** are posted to the investigation thread, forwarded to Mattermost/Slack, and stored as [agent memories](/core-features/agent-memory) for future recall

## Choosing an Agent Runtime

Alga supports three categories of AI agent:

| Option | Best For | Setup |
|--------|----------|-------|
| **[Hermes](/integrations/hermes)** | Teams using Nous Research's Hermes Agent platform | Install the Python plugin, configure SSE + REST |
| **[OpenClaw](/integrations/openclaw)** | Teams using the OpenClaw personal AI assistant | Install the TypeScript channel plugin |
| **[Custom SDK](/integrations/agent-sdks)** | Full control — build your own agent in Go, JS, Python, or Rust | Use the SSE + REST agent API directly |

Hermes and OpenClaw are **peer alternatives** — both connect to the same agent API and have access to the same backend toolset. The scheduler is agent-type-agnostic: it picks whichever online agent has the right capabilities and scope.

::: tip Running multiple agents
You can register multiple agents with different scopes. For example, one agent scoped to `labels.team=dba` for database expertise, and another catch-all (`scope=all`) for everything else. The scheduler prefers label-specific agents over catch-all.
:::

## Agent Setup

### Creating an Agent Token

Regardless of which runtime you choose, every agent needs a token:

1. In the Alga web UI, go to **Integrations → Agents → Add agent**
2. Choose the **agent type** (`hermes`, `openclaw`, or `other`)
3. Select **capabilities** (at minimum: `investigate`)
4. Set the **scope** — `all` (default) or `labels` with label selectors
5. **Set as default** if this agent should receive automated dispatch traffic
6. Save — **copy the token immediately** (`alga_agent_...`), it's shown only once

### Connecting a Runtime

For detailed setup instructions, see the integration guide for your chosen runtime:

- **[Hermes setup →](/integrations/hermes#setup-guide)**
- **[OpenClaw setup →](/integrations/openclaw)**
- **[Custom SDK setup →](/integrations/agent-sdks)**

## Agent Types

| Type | Description |
|------|-------------|
| `hermes` | Hermes agent via the Alga platform adapter plugin (the default type) |
| `openclaw` | OpenClaw channel plugin agent |
| `other` | Custom agent using the SSE + REST API |

The agent type is used for display (avatar icon, status string) and is informational. Capabilities and scope are what actually gate agent behavior.

## Capabilities

Capabilities gate **what** an agent can do. Assign them during token creation or update via `PUT /api/v1/agent-tokens/{id}`:

| Capability | Allows |
|-----------|--------|
| `investigate` | Receive investigations, resolve/reopen alerts, set outcomes, search knowledge, query alerts/services/on-call |
| `communicate` | Publish status updates, handle communications tasks |
| `command` | Incident command — set priority/severity, trigger escalation, mitigate/resolve incidents, assign ICS roles |

List available capabilities:

```bash
GET /api/v1/agent/capabilities
```

## Scope

Scope gates **which** investigations an agent receives:

| Scope | Behavior |
|-------|---------|
| `all` | Receive all investigations (default) |
| `labels` | Only receive investigations whose alert labels match the configured selectors |

When scope is `labels`, provide `label_selectors`:

```json
{
  "scope": "labels",
  "label_selectors": [
    { "field": "labels.namespace", "op": "exact", "value": "production" }
  ]
}
```

## Agent Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/agent-tokens` | List agent tokens (with online presence) |
| `POST` | `/api/v1/agent-tokens` | Create agent token |
| `PUT` | `/api/v1/agent-tokens/{id}` | Update agent config (scope, capabilities, enabled) |
| `POST` | `/api/v1/agent-tokens/{id}/regenerate` | Regenerate the bearer token |
| `POST` | `/api/v1/agent-tokens/{id}/set-default` | Designate as default dispatch target |
| `DELETE` | `/api/v1/agent-tokens/{id}` | Revoke agent token |

### Enabling / Disabling

Disabled agents are excluded from scheduling and their SSE connections are rejected. Re-enable any time without regenerating the token:

```bash
PUT /api/v1/agent-tokens/{id}
{ "enabled": false }
```

### Agent DM Chat

Operators can send direct messages to any agent for ad-hoc queries without a formal investigation:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/agent-tokens/{id}/chat/messages` | List DM messages |
| `POST` | `/api/v1/agent-tokens/{id}/chat/messages` | Send a message |
| `POST` | `/api/v1/agent-tokens/{id}/chat/typing` | Typing indicator |

## Investigation Lifecycle

### Alert Investigation

| Status | Description |
|--------|-------------|
| `pending` | Waiting for an available agent |
| `assigned` | Claimed by an agent, awaiting start |
| `investigating` | Agent is actively analyzing |
| `complete` | Investigation finished successfully |
| `promoted` | Promoted to an incident investigation (irreversible from the alert side) |
| `failed` | Agent reported an error |
| `timed_out` | Agent did not respond within `INVESTIGATION_TIMEOUT` |
| `cancelled` | Manually cancelled by operator |
| `paused` | Temporarily paused (waiting for external events) |

Investigations can be **requeued** back to `pending` (e.g., after agent disconnect or dispatch failure). Completing an investigation that is already in a terminal status (`complete`, `failed`, `cancelled`, `timed_out`, `promoted`) is a no-op — the completion is idempotent.

### Incident Investigation

| Status | Description |
|--------|-------------|
| `pending` | Waiting for an available agent |
| `assigned` | Claimed by an agent, awaiting start |
| `investigating` | Agent is actively analyzing |
| `complete` | Investigation finished successfully |
| `cancelled` | Cancelled by operator or commander |
| `paused` | Temporarily paused |
| `coordinating` | Commander-owned investigation coordinating child investigations (excluded from normal scheduling) |

### Promotion

An alert investigation can be **promoted** to an incident investigation. Promotion is irreversible from the alert side — the alert investigation enters the terminal `promoted` status and its work is handed off to the incident. Reopening the linked alert does not demote a promoted investigation back into active duty.

### Manual Assignment

Operators can manually assign investigations to a specific agent:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `PATCH` | `/api/v1/alert-investigations/{id}/assign` | Assign alert investigation to agent |
| `PATCH` | `/api/v1/incident-investigations/{id}/assign` | Assign incident investigation to agent |

### Dead-Lettered Investigations

Investigations that exhaust all retry attempts (through the RabbitMQ retry topology) are dead-lettered. Admins can inspect them:

```bash
GET /api/v1/investigations/dead-lettered
```

This requires admin access (`rbac.AdminAccess`). Dead-lettered investigations include the final error message and retry count for debugging.

## Health Tracking and Circuit Breaking

The `AgentHealthTracker` monitors per-agent success and failure rates using a sliding window. Agents with consistently high failure rates are **circuit-broken** — the scheduler skips them until their health recovers. This prevents repeatedly assigning investigations to a misbehaving agent.

Health scores are visible on the Agents page. The circuit breaker threshold prevents scheduling to agents below a minimum health score.

## Playbook Enrichment

When [playbooks](/core-features/playbooks) are configured with label selectors, the scheduler automatically matches them to investigations and includes their steps in the dispatch prompt. This gives the agent runbook-style guidance alongside the alert context — without the agent needing to search for it.

## Context Injected Into Dispatch Prompts

When the scheduler dispatches an investigation, it builds a rich prompt containing:

- **Alert details** — name, labels, annotations, severity, summary, description
- **Correlation key** and primary alert fingerprint
- **Triage enrichment** — the triage decision and reasoning
- **Shared knowledge** — matching [knowledge notes](/core-features/knowledge-base) (previews)
- **Playbook steps** — matched by label selectors
- **Episodic memory** — past investigations with the same correlation key
- **Agent memories** — [semantically-relevant memories](/core-features/agent-memory) from past investigations
- **Ops team context** — who is on call, escalation contacts
- **Role-specific instructions** — commander/responder/communicator behavioral contracts

## What Agents Can Do

Agents interact with Alga through `alga_*` tools. The toolset depends on the plugin:

- **Hermes plugin** — 31 tools
- **OpenClaw plugin** — 32 tools (adds memory and peer-ask tools)

Both plugins share the same backend executor (`AgentToolExecutor`). Tools fall into categories:

| Category | Representative Tools |
|----------|---------------------|
| Alert lifecycle | `alga_resolve_alert`, `alga_reopen_alert`, `alga_set_outcome`, `alga_cancel_investigation`, `alga_pause_investigation` |
| Incident command | `alga_promote_to_incident`, `alga_set_incident_priority`, `alga_set_incident_severity`, `alga_trigger_escalation`, `alga_mitigate_incident`, `alga_resolve_incident` |
| Coordination | `alga_dispatch_task`, `alga_complete_task`, `alga_synthesize_findings`, `alga_publish_status_update`, `alga_post_handoff` |
| Knowledge | `alga_search_knowledge`, `alga_get_knowledge`, `alga_create_knowledge` |
| Context queries | `alga_list_alerts`, `alga_get_incident_context`, `alga_get_incident_timeline`, `alga_list_services`, `alga_who_is_on_call` |
| Triage feedback | `alga_triage_feedback` |
| Memory (OpenClaw) | `alga_search_memories`, `alga_create_memory` |
| Peer collaboration (OpenClaw) | `alga_peer_ask` |

See the [Hermes](/integrations/hermes#the-31-agent-tools) or [OpenClaw](/integrations/openclaw) integration guide for the complete tool reference.

### Incident Role Boundaries

During an incident, agents are bound by their ICS role:

| Role | Allowed Actions |
|------|----------------|
| **Incident Commander** | Priority, escalation, mitigation, resolution, resolution docs, role assignment, task dispatch |
| **Responder** | Investigation updates, severity, outcome, pause/cancel, complete investigate/verify/mitigate tasks |
| **Communications Lead** | Publish status updates, complete communicate-kind tasks |

The backend enforces these boundaries server-side. If an agent attempts an action outside its role, it receives a clear 403 error with the `required_capability`.

## Agent REST API

All agent API endpoints require the agent bearer token and are rate-limited per-agent:

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/agent/events` | SSE stream for investigation dispatch, lifecycle signals, peer-ask, coordination |
| `POST /api/v1/agent/messages` | Send text, tool calls, or `inv_tool` commands |
| `PUT /api/v1/agent/messages/{id}` | Edit a previous message |
| `DELETE /api/v1/agent/messages/{id}` | Delete a message |
| `POST /api/v1/agent/drafts` | Streaming draft preview |
| `POST /api/v1/agent/typing` | Typing indicator |
| `POST /api/v1/agent/heartbeat` | Renew SSE presence |
| `GET/POST /api/v1/agent/alerts` | Alert queries and actions |
| `GET /api/v1/agent/alerts/{fp}` | Get a specific alert by fingerprint |
| `GET/POST /api/v1/agent/memories` | Agent memory CRUD |
| `GET /api/v1/agent/peer-ask` | [Peer ask](/core-features/peer-ask) — agent-to-agent questions |
| `GET /api/v1/agent/incidents/{id}` | Get incident context |
| `PATCH /api/v1/agent/incidents/{id}` | Update incident |
| `GET /api/v1/agent/incidents/{id}/timeline` | Get incident timeline |
| `POST /api/v1/agent/incidents/{id}/timeline` | Add timeline entry |
| `GET /api/v1/agent/incidents/{id}/tasks` | List coordination tasks |
| `GET /api/v1/agent/services` | List services |
| `GET /api/v1/agent/on-call/current` | Who is on call now |
| `GET /api/v1/agent/playbooks` | Matching playbooks |
| `GET /api/v1/agent/knowledge` | Read shared knowledge notes |

## Investigation Threads

Each investigation has an **owner-thread** chat that supports real-time updates via SSE. Operators and agents communicate in this thread during an active investigation.

- **SSE stream**: `GET /api/v1/agent/events` delivers real-time investigation dispatch, lifecycle signals, and thread messages
- **Typing indicators**: agents and operators emit typing events for live feedback
- **Cross-provider sync**: thread messages are synchronized bidirectionally with Slack and Mattermost — replies in external channels appear in the Alga thread and vice versa

### Agent Message Types

Agents send structured messages through the thread. Each message has a type:

| Type | Description |
|------|-------------|
| `text` | Free-form text message |
| `tool_call` | Agent invoking an `alga_*` tool |
| `inv_tool` | Investigation-scoped tool command |
| `incident_summary` | Summary of incident state |
| `triage_response` | Triage feedback from the agent |
| `command_decision` | Commander decision (escalation, mitigation, resolution) |
| `status_update` | Status update for external communication |

## Scheduler

The investigation scheduler is **leader-elected** via Valkey (only one replica runs the tick loop). It ticks every **5 seconds** and runs a **Filter → Score → Bind** pass:

1. **Filter** — list online agents (presence TTL), remove circuit-broken agents, list pending investigations, apply backoff and scope filters
2. **Score** — rank candidate agents for each investigation:
   - **Specificity** — label-matched agents (scope `labels` with matching `label_selectors`) score higher than catch-all (`scope: all`)
   - **Load** — least busy agent wins (fewest active investigations)
   - **Health** — highest success rate (from `AgentHealthTracker` sliding window)
3. **Bind** — atomically claim the investigation for the best candidate and dispatch via SSE

Concurrency is limited per-agent by `MAX_CONCURRENT_INVESTIGATIONS`. The circuit breaker (`AgentHealthTracker`) skips agents with consistently high failure rates until their health recovers.

### Stuck Investigation Escalation

If an investigation exceeds `multiplier * INVESTIGATION_TIMEOUT` without completing, the scheduler escalates it via the configured escalation policy.

### Agent Disconnect Grace

When an agent disconnects, the scheduler waits `AGENT_DISCONNECT_GRACE` (default 45s) before requeuing any investigations that were picked up (status `assigned` or `investigating`) by that agent back to `pending`.

## Scheduler Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `CORRELATION_WINDOW` | `0` (disabled) | Time window for grouping related alerts |
| `CORRELATION_COOLDOWN_TTL` | `30m` | Cooldown before a new investigation opens on the same key |
| `INVESTIGATION_TIMEOUT` | `10m` | Maximum investigation duration before timeout |
| `MAX_CONCURRENT_INVESTIGATIONS` | `3` | Per-agent capacity limit |
| `SCHEDULER_LEADER_TTL` | `15s` | Leader election lease TTL |
| `AGENT_PRESENCE_TTL` | `90s` | Agent presence TTL in Valkey |
| `AGENT_DISCONNECT_GRACE` | `45s` | Grace period before requeuing picked-up investigations on disconnect |
| `STALE_ALERT_THRESHOLD` | `15m` | Alert age before the stale sweep considers it uninvestigated |
| `STALE_ALERT_SWEEP_INTERVAL` | `5m` | How often the stale sweep runs |

## High Availability

With Valkey/Redis configured:

- **Agent presence** is tracked across replicas — any replica can see which agents are online
- **Scheduler leader election** ensures a single tick runner across the cluster (via Valkey leader lease)
- **Cross-replica SSE fan-out** — agent events published on one replica reach agents connected to any replica
- **Atomic investigation claim** prevents double-dispatch in multi-replica deployments
- **Disconnect grace period** (`AGENT_DISCONNECT_GRACE`) with Valkey-fenced reset — after the grace period, `assigned` and `investigating` work is requeued to `pending`

## See Also

- [Hermes Integration](/integrations/hermes) — Hermes agent setup and 31-tool reference
- [OpenClaw Integration](/integrations/openclaw) — OpenClaw agent setup
- [Agent SDKs](/integrations/agent-sdks) — build a custom agent
- [Agent Memory](/core-features/agent-memory) — vector-searched agent memories
- [Peer Ask](/core-features/peer-ask) — agent-to-agent collaboration
- [Knowledge Base](/core-features/knowledge-base) — operator-authored notes for agents
- [Triage](/core-features/triage) — how alerts are classified before investigation
- [Playbooks](/core-features/playbooks) — enrich investigations with runbook steps
