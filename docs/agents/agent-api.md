---
title: Agent API & Tool Reference
description: The shared contract every Alga agent runtime speaks — SSE + REST flow, owner-scoped threads, message semantics, presence, the canonical alga_* tool catalog, and incident role boundaries.
---

# Agent API & Tool Reference

Every agent runtime — the native [Alga Agent](/agents/alga-agent), the [Hermes](/agents/hermes) and [OpenClaw](/agents/openclaw) plugins, and anything built on the [Agent SDKs](/agents/agent-sdks) — speaks the same contract described here. The scheduler is agent-type-agnostic: any online agent with the right capabilities and scope can win a dispatch. Runtime pages cover only what is unique to each runtime; this page is the single source of truth for everything they share.

## Connection Model

```
┌─────────────────────────────────────────────────────────────┐
│                        Alga Backend                          │
│  /api/v1/agent/events ◀── SSE (dispatch, chat, lifecycle)   │
│  /api/v1/agent/messages ◀── REST POST (agent → thread)      │
│  /api/v1/agent/heartbeat ◀── REST POST (presence)           │
└─────────────────────────────────────────────────────────────┘
                              ▲
                              │ SSE + REST (bearer token)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Agent Runtime                            │
│  SSE listener + REST sender bridging to owner-scoped threads │
│  alga_* tools (see catalog below)                            │
│  chat_id = "alert_42", "incident_coord_12", etc.            │
└─────────────────────────────────────────────────────────────┘
```

**The flow:** Alert → Correlator → Scheduler → agent (SSE dispatch) → agent reasons and calls `alga_*` tools → Alga executes mutations and posts confirmations to the investigation thread → findings forwarded to Mattermost/Slack.

The agent authenticates with `Authorization: Bearer alga_agent_...` on every call. See [Agents Overview](/agents/) for creating tokens, capabilities, and scope.

## Owner-Scoped Threads

Alga dispatches work using owner-scoped chat IDs that each runtime maps to its own sessions:

| Chat ID Pattern     | Meaning                                       |
| ------------------- | --------------------------------------------- |
| `alert_42`          | Alert investigation thread for alert #42      |
| `incident_coord_12` | Incident coordination thread for incident #12 |
| `incident_inv_12`   | Incident investigation working thread         |
| `alga_dm`           | Private operator DM chat                      |

Bare numbers passed as an `investigation_id` argument are treated as alert numbers; incident tools build the `incident_coord_<number>` chat ID automatically.

## Message Flow

### Alga → Agent (SSE dispatch)

When an alert is dispatched, the agent receives an SSE `message` event with `trigger: "dispatch"` containing the full investigation prompt — alert details, labels, annotations, shared knowledge, triage enrichment, playbook steps, and role-specific instructions.

```json
{
  "type": "message",
  "chat_id": "alert_42",
  "text": "What's the status of the alert?",
  "sender_id": "user_id_abc",
  "sender_name": "John Doe"
}
```

### Agent → Alga (REST)

The agent responds by posting messages and executing tools via REST:

```json
{
  "chat_id": "alert_42",
  "text": "I'm investigating the high CPU alert now."
}
```

### Thread replies

Both directions support Telegram/Discord-style message replies. When a thread message is a reply to an earlier message, the SSE event includes `reply_to_message_id` and `reply_to_text`, and runtimes inject the replied-to context into the agent prompt.

### Internal messages

Messages starting with 🔒 are **not forwarded** to the agent — use this prefix for operator-only context that shouldn't reach the agent.

### Slash commands

- `/stop` — cancels the agent's in-progress generation for that thread
- All other messages are appended to the transcript as observation context without interrupting

## Heartbeat & Presence

Runtimes maintain presence by posting to `/api/v1/agent/heartbeat` roughly every 30 seconds to keep the Valkey presence lease alive; the backend also sends a 30s SSE keepalive comment that renews presence. Only online agents are eligible for dispatch. If the connection drops:

- **Delegated** work is reset immediately for re-dispatch
- **Investigating** work gets a grace period (`AGENT_DISCONNECT_GRACE`, default 45s) before reset
- The scheduler may circuit-break agents with sustained high failure rates

Reconnect strategy is runtime-specific (for example, OpenClaw uses a fixed 5s delay; Hermes uses exponential backoff). See each runtime page for details.

## The alga_* Tool Catalog

Each tool either sends an `inv_tool` command via `POST /api/v1/agent/messages` or makes a direct REST query to a dedicated agent endpoint. The **Runtimes** column marks which runtimes expose each tool: **All** = native Alga Agent, Hermes, and OpenClaw; **H** = Hermes-exclusive; **O** = OpenClaw-exclusive.

### Alert & Investigation

| Tool                        | Runtimes | Description                                                                                               |
| --------------------------- | -------- | --------------------------------------------------------------------------------------------------------- |
| `alga_resolve_alert`        | All      | Resolve an alert and close the investigation (optionally record root cause + resolution).                 |
| `alga_reopen_alert`         | All      | Reopen a resolved alert and resume the investigation.                                                     |
| `alga_promote_to_incident`  | All      | Promote an alert investigation to a full incident (borrows the investigation summary as the description). |
| `alga_set_outcome`          | All      | Record root cause and/or resolution without resolving — use for progressive documentation.                |
| `alga_cancel_investigation` | All      | Cancel the investigation (false positive, transient, not actionable). Requires a reason.                  |
| `alga_pause_investigation`  | All      | Pause investigation when waiting for external events or human input. Requires a reason.                   |
| `alga_list_alerts`          | All      | Query the broader alert landscape (filter by status, severity, search text).                              |
| `alga_triage_feedback`      | All      | Confirm or override a triage decision to improve future accuracy.                                         |
| `alga_assign_investigation` | O        | Reassign the current investigation to a different agent. Only the currently assigned agent can reassign.  |

### Knowledge

| Tool                    | Runtimes | Description                                                                                                      |
| ----------------------- | -------- | ---------------------------------------------------------------------------------------------------------------- |
| `alga_search_knowledge` | All      | Search shared knowledge notes (runbooks, known issues, service owners, facts) — returns 200-char previews.       |
| `alga_get_knowledge`    | All      | Fetch the full body of a knowledge note by ID.                                                                   |
| `alga_create_knowledge` | All      | Create a reusable knowledge note from investigation findings (kinds: runbook, known_issue, service_owner, fact). |

### Memory

| Tool                   | Runtimes | Description                                                                                      |
| ---------------------- | -------- | ------------------------------------------------------------------------------------------------ |
| `alga_search_memories` | O        | Search agent-scoped memories from past investigations (semantic vector search).                  |
| `alga_create_memory`   | O        | Create an agent memory for future investigations — persist useful findings, fixes, and insights. |

::: tip Memory vs Knowledge
`alga_search_knowledge` queries the **shared, operator-curated knowledge base**. `alga_search_memories` queries the **agent's own episodic memories** — things it learned during past investigations and saved for itself.
:::

### Peer Collaboration

| Tool            | Runtimes | Description                                                                           |
| --------------- | -------- | ------------------------------------------------------------------------------------- |
| `alga_peer_ask` | O        | Ask another agent for help — creates a peer-ask request the target receives over SSE. |

Hermes does not register a named peer-ask tool, but Hermes agents can still use the REST API directly.

### Incident Command (Commander-Only)

| Tool                                | Runtimes | Description                                                                                                                                       |
| ----------------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `alga_set_incident_priority`        | All      | Set incident priority (P1–P5). Affects SLA targets.                                                                                               |
| `alga_set_incident_severity`        | All      | Set incident severity (critical, high, warning, info).                                                                                            |
| `alga_trigger_escalation`           | All      | Trigger escalation for the incident — notifies on-call responders and escalation contacts.                                                        |
| `alga_request_status_update`        | O        | Request a status update from incident responders — sends a notification asking for a progress report.                                             |
| `alga_mitigate_incident`            | All      | Mark an incident as mitigated (impact contained, root cause may not be fully resolved).                                                           |
| `alga_resolve_incident`             | All      | Resolve an incident (requires all five resolution docs).                                                                                          |
| `alga_set_incident_resolution_docs` | All      | Stage structured resolution documents (summary, impact, actions, root cause, resolution) without resolving.                                       |
| `alga_begin_triage`                 | All      | Move incident from `detected` to `triaging`.                                                                                                      |
| `alga_promote_incident`             | All      | Promote incident from `triaging` to `active`.                                                                                                     |
| `alga_assign_incident_role`         | All      | Assign ICS command roles (incident_commander, operations_lead, communications_lead, scribe, etc.) to users or agents.                             |
| `alga_post_handoff`                 | All      | Commander-facing **final** handoff only — see warning below.                                                                                      |
| `alga_publish_status_update`        | All      | Publish a public status update (`investigating`, `identified`, `monitoring`, `resolved`). The only path that creates a Status Updates card entry. |

::: warning `alga_post_handoff` activates other agents
Every `alga_post_handoff` call **wakes up teammate agents** (commander, communicator) by forwarding the message to them, which can interrupt their current work and cause ping-pong loops. Reserve it for the single structured commander handoff that happens **after recovery is verified** and a `monitoring` status update has already been published via `alga_publish_status_update`. For status milestones during active work, always use `alga_publish_status_update` instead.
:::

### Coordination Tasks (Hermes task-driven model)

| Tool                       | Runtimes | Description                                                                                |
| -------------------------- | -------- | ------------------------------------------------------------------------------------------ |
| `alga_dispatch_task`       | H        | Dispatch a typed coordination task to a role (investigate, communicate, verify, mitigate). |
| `alga_complete_task`       | H        | Complete a coordination task with a typed result.                                          |
| `alga_list_tasks`          | H        | List coordination tasks (commander tracks dispatched progress).                            |
| `alga_synthesize_findings` | H        | Synthesize findings from completed child investigations into the incident conclusion.      |

See [Coordination](/incident-management/coordination) for the multi-agent incident coordination model.

### Incident Query

| Tool                         | Runtimes | Description                                                                                   |
| ---------------------------- | -------- | --------------------------------------------------------------------------------------------- |
| `alga_get_incident_context`  | All      | Get full incident context (status, severity, timeline, roles, linked alerts/investigations).  |
| `alga_get_incident_timeline` | All      | Get the timeline of events for an incident.                                                   |
| `alga_add_incident_timeline` | All      | Log a custom entry to the incident timeline (progress, finding, action, resolution, comment). |

### Utility

| Tool                  | Runtimes | Description                                       |
| --------------------- | -------- | ------------------------------------------------- |
| `alga_list_services`  | All      | List all registered services with current status. |
| `alga_who_is_on_call` | All      | Get the current on-call person for each schedule. |

## Incident Role Boundaries

The Alga backend enforces incident role boundaries server-side. Runtimes mirror these rules in their tool descriptions so the model doesn't waste calls on tools outside its role:

| Active Role             | Allowed Actions                                                                                                                |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| **Incident Commander**  | Priority, escalation, mitigation, resolution, resolution docs, triage/promote, role assignment, status updates, tasks, handoff |
| **Responder**           | Investigation updates, severity, outcome, pause/cancel, timeline entries, complete investigate/verify/mitigate tasks           |
| **Communications Lead** | Publish public status updates, add timeline entries, complete communicate-kind tasks                                           |

::: warning Resolution Requirements
Incident resolution requires five structured artifacts: `summary`, `impact_assessment`, `actions_taken`, `root_cause`, and `resolution`. The `root_cause` and `resolution` sections are independently mandatory. A commander supplies them inline to `resolve_incident` or stages them with `set_incident_resolution_docs` first.
:::

## Shared Troubleshooting

### Agent not receiving investigations

```bash
# Verify Alga is running
curl http://localhost:8080/health

# Check the SSE endpoint (should keep the connection open)
curl -N -H "Authorization: Bearer alga_agent_xxxxxxxxx" \
  http://localhost:8080/api/v1/agent/events
```

Common causes:

- **Agent token not set as default** — only the default agent receives automated dispatch traffic that no label-targeted agent claims
- **Agent offline** — verify the runtime is running and the SSE connection is active (**Agents** should show a green dot)
- **Scope mismatch** — if scope is `labels`, the alert labels must match the configured selectors
- **Capability missing** — the agent needs the `investigate` capability to receive dispatches
- **Circuit-broken** — check if the agent has a high failure rate (visible in the Agents page)

### Auth failed

Ensure the `Authorization: Bearer <token>` header uses the full `alga_agent_...` token. The token is validated against a stored HMAC hash with constant-time comparison.

### Messages starting with 🔒 not forwarded

This is by design — the 🔒 prefix marks operator-internal messages that should not reach the agent.

### Investigation completes but alert isn't resolved

The agent must explicitly call `alga_resolve_alert` to resolve alerts. Text messages alone don't resolve alerts — they only document findings. Check the investigation thread for tool call results.

## See Also

- [Agents Overview](/agents/) — tokens, capabilities, scope, presence, private chat
- [Alga Agent](/agents/alga-agent), [Hermes](/agents/hermes), [OpenClaw](/agents/openclaw) — runtime-specific setup
- [Agent SDKs](/agents/agent-sdks) — build a custom agent
- [AI Investigation](/core-features/investigation) — the dispatch pipeline and scheduler
- [Agent REST API](/api-reference/#agent-rest-api) — the full endpoint surface
