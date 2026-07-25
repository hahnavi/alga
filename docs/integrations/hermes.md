---
title: Hermes Agent
description: Connect a Nous Research Hermes Agent to Alga for autonomous AI alert investigation with 31 alga_* tools, ICS incident roles, and multi-agent coordination.
---

# Hermes Agent

[Hermes Agent](https://github.com/nousresearch/hermes-agent) is an autonomous AI agent platform by Nous Research. Alga connects to a Hermes gateway via a self-contained Python plugin, allowing Hermes to act as an AI SRE investigator — receiving alert dispatches over SSE, reasoning about root causes, and taking lifecycle actions (resolve, escalate, promote to incident) through 31 `alga_*` tools.

Hermes and [OpenClaw](/integrations/openclaw) are **alternative, peer agent runtimes** — both plug into the same Alga agent API. You can run either one, or both as different agent tokens. The scheduler is agent-type-agnostic: any online agent with the right capabilities and scope can win a dispatch.

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│                        Alga Backend                          │
│  /api/v1/agent/events ◀── SSE (dispatch, chat, lifecycle)   │
│  /api/v1/agent/messages ◀── REST POST (agent → thread)      │
│  /api/v1/agent/heartbeat ◀── REST POST (presence)           │
└─────────────────────────────────────────────────────────────┘
                              ▲
                              │ SSE + REST (httpx)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Hermes Gateway                           │
│  ~/.hermes/plugins/alga-platform/                            │
│  - Platform adapter (SSE + REST bridge)                      │
│  - 31 agent tools under the "alga" toolset                   │
│  - chat_id = "alert_42", "incident_coord_12", etc.          │
└─────────────────────────────────────────────────────────────┘
```

**The flow:** Alert → Correlator → Scheduler → Hermes (SSE dispatch) → Hermes reasons and calls `alga_*` tools → Alga executes mutations and posts confirmations to the investigation thread → Findings forwarded to Mattermost/Slack.

## Prerequisites

- Hermes Agent installed (`~/.hermes/` exists)
- `httpx>=0.27` Python package (`pip install 'httpx>=0.27'`)
- Alga backend running and reachable from the Hermes host

## Setup Guide

### Step 1: Create an Agent Token in Alga

1. In the Alga web UI, go to **Integrations → Agents → Add agent**
2. Choose **Hermes Agent** as the agent type
3. Select the capabilities you need (at minimum: `investigate`)
4. Set the scope (`all` for catch-all, or `labels` to restrict to specific alert labels)
5. **Check "Set as default"** if this agent should receive automated investigation traffic
6. Save — **copy the token immediately**, it's shown only once (`alga_agent_...`)

::: tip
The `hermes` agent type is the default in Alga. If you leave the type unspecified or enter an unrecognized value, it normalizes to `hermes`.
:::

### Step 2: Install the Plugin

```bash
cd integrations/alga-hermes-agent-plugin
bash install.sh
```

### Step 3: Enable the Plugin

```bash
hermes plugins enable alga-platform
```

### Step 4: Configure Credentials

Add to `~/.hermes/.env`:

```bash
ALGA_SERVER_URL=http://alga:8080
ALGA_AGENT_TOKEN=alga_agent_xxxxxxxxxxxxxxxx
```

Or in `~/.hermes/config.yaml`:

```yaml
platforms:
  alga:
    enabled: true
    extra:
      server_url: "http://alga:8080"
    token: "${ALGA_AGENT_TOKEN}"
```

### Step 5: Enable the Toolset and Restart

```bash
hermes tools enable alga
hermes gateway restart
```

### Interactive Setup (Alternative)

The plugin integrates with `hermes gateway setup` — select **"Alga"** from the platform list and it will prompt for the server URL and agent token interactively.

## Configuration Reference

| Variable | Required | Description |
|----------|----------|-------------|
| `ALGA_SERVER_URL` | Yes | Alga backend base URL (no trailing slash), e.g. `http://alga:8080` |
| `ALGA_AGENT_TOKEN` | Yes | The `alga_agent_...` bearer token from Alga |
| `ALGA_HOME_CHANNEL` | No | Private operator DM chat ID (default: `alga_dm`) |
| `ALGA_ALLOWED_USERS` | No | Comma-separated user IDs allowed to use the channel (default: allow all) |
| `ALGA_ALLOW_ALL_USERS` | No | Set to `true` to allow all users (default) |

## Backend Configuration (Alga Side)

On the Alga backend, the Hermes integration is configured via:

| Variable | Description |
|----------|-------------|
| `HERMES_PLATFORM_URL` | URL of the Hermes platform (stored encrypted at rest in `IntegrationConfig`) |
| `HERMES_PLATFORM_TOKEN` | Platform token for Hermes (stored encrypted at rest in `IntegrationConfig`) |

### Agent Capabilities

When creating a Hermes agent in Alga, you assign capabilities:

| Capability | Description |
|------------|-------------|
| `investigate` | Receive automated alert investigation dispatches |
| `communicate` | Participate in incident communications |
| `command` | Take incident commander actions |

### Agent Scope

| Scope | Description |
|-------|-------------|
| `all` | Catch-all — eligible for any alert dispatch |
| `label_selectors` | Restricted to alerts matching specific label selectors |

## What the Plugin Provides

| Component | Description |
|-----------|-------------|
| **Platform adapter** | SSE listener + REST sender bridging Hermes to Alga's owner-scoped threads |
| **31 agent tools** | The full `alga_*` toolset for alert lifecycle, incident management, knowledge, and coordination |
| **Setup wizard** | Interactive configuration via `hermes gateway setup` |
| **Platform hint** | Injected into the LLM system prompt with Alga-specific guidance and role instructions |

## Owner-Scoped Threads

Alga dispatches work using owner-scoped chat IDs that the plugin maps to Hermes sessions:

| Chat ID Pattern | Meaning |
|-----------------|---------|
| `alert_42` | Alert investigation thread for alert #42 |
| `incident_coord_12` | Incident coordination thread for incident #12 |
| `incident_inv_12` | Incident investigation working thread |
| `alga_dm` | Private operator DM chat |

## Message Flow

### Alga → Hermes (SSE dispatch)

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

### Hermes → Alga (REST)

Hermes responds by posting messages and executing tools via REST:

```json
{
  "chat_id": "alert_42",
  "text": "I'm investigating the high CPU alert now."
}
```

### Thread replies

Both directions support Telegram/Discord-style message replies. When a thread message is a reply to an earlier message, the SSE event includes `reply_to_message_id` and `reply_to_text`, and Hermes injects a `[Replying to: "..."]` prefix into the agent prompt.

### Internal messages

Messages starting with 🔒 are **not forwarded** to Hermes — use this prefix for operator-only context that shouldn't reach the agent.

### Slash commands

- `/stop` — cancels the agent's in-progress generation for that thread
- All other messages are appended to the transcript as observation context without interrupting

## The 31 Agent Tools

The plugin registers 31 tools under the `"alga"` toolset. Each tool either sends an `inv_tool` command via `POST /api/v1/agent/messages` or makes a direct REST query.

### Alert & Investigation Tools

| Tool | Description |
|------|-------------|
| `alga_resolve_alert` | Resolve an alert and close the investigation (optionally record root cause + resolution) |
| `alga_reopen_alert` | Reopen a resolved alert and resume the investigation |
| `alga_promote_to_incident` | Promote an alert investigation to a full incident |
| `alga_set_outcome` | Record root cause and/or resolution without resolving |
| `alga_cancel_investigation` | Cancel the investigation with a reason |
| `alga_pause_investigation` | Pause investigation when waiting for external events |
| `alga_list_alerts` | Query the broader alert landscape (filter by status, severity, search) |
| `alga_triage_feedback` | Confirm or override a triage decision to improve accuracy |

### Knowledge Tools

| Tool | Description |
|------|-------------|
| `alga_search_knowledge` | Search knowledge notes (runbooks, known issues, service owners, facts) — returns 200-char previews |
| `alga_get_knowledge` | Fetch the full body of a knowledge note by ID |
| `alga_create_knowledge` | Create a reusable knowledge note from investigation findings |

### Incident Command Tools (Commander-Only)

| Tool | Description |
|------|-------------|
| `alga_set_incident_priority` | Set incident priority (P1–P5) |
| `alga_set_incident_severity` | Set incident severity (critical, high, warning, info) |
| `alga_trigger_escalation` | Trigger escalation for the incident |
| `alga_dispatch_task` | Dispatch a typed coordination task to a role (`investigate`, `communicate`, `verify`, `mitigate`) |
| `alga_complete_task` | Complete a coordination task with a typed result |
| `alga_list_tasks` | List coordination tasks (commander tracks dispatched progress) |
| `alga_synthesize_findings` | Synthesize findings from completed child investigations into the incident conclusion |
| `alga_mitigate_incident` | Mark an incident as mitigated |
| `alga_resolve_incident` | Resolve an incident (requires all five resolution docs) |
| `alga_set_incident_resolution_docs` | Stage resolution documents without resolving |
| `alga_begin_triage` | Move incident to `triaging` status |
| `alga_promote_incident` | Promote incident from `triaging` to `active` |
| `alga_assign_incident_role` | Assign ICS command roles to users or agents |
| `alga_post_handoff` | Final commander handoff only — **warning:** activates other agents, can cause ping-pong |
| `alga_publish_status_update` | Publish a public status update (`investigating`, `identified`, `monitoring`, `resolved`) |

### Incident Query Tools

| Tool | Description |
|------|-------------|
| `alga_get_incident_context` | Get full incident context (status, severity, timeline, roles) |
| `alga_get_incident_timeline` | Get the timeline of events for an incident |
| `alga_add_incident_timeline` | Log a custom note or status update to the incident timeline |

### Utility Tools

| Tool | Description |
|------|-------------|
| `alga_list_services` | List all registered services with current status |
| `alga_who_is_on_call` | Get the current on-call person for each schedule |

## Incident Role Boundaries

The Alga backend enforces incident role boundaries server-side. The plugin mirrors these rules in its tool descriptions so the model doesn't waste calls on tools outside its role:

| Active Role | Allowed Actions |
|-------------|----------------|
| **Incident Commander** | Priority, escalation, mitigation, resolution, resolution docs, triage/promote, role assignment, task dispatch/track/synthesize |
| **Responder** | Investigation updates, severity, outcome, pause/cancel, complete investigate/verify/mitigate tasks |
| **Communications Lead** | Publish public status updates, complete communicate-kind tasks |

::: warning Resolution Requirements
Incident resolution requires five structured artifacts: `summary`, `impact_assessment`, `actions_taken`, `root_cause`, and `resolution`. The `root_cause` and `resolution` sections are independently mandatory. A commander supplies them inline to `resolve_incident` or stages them with `set_incident_resolution_docs` first.
:::

## Multi-Agent Incident Coordination

During an incident, multiple Hermes agents (commander, communicator, responder) share the incident coordination thread and @mention each other. Coordination is **task-driven**:

1. **Commander** decomposes the incident into typed tasks via `alga_dispatch_task` (kinds: `investigate`, `communicate`, `verify`, `mitigate`)
2. Dispatched tasks **wake the assigned agent** through the `coordination_task_dispatched` SSE event
3. The agent acts on the task and completes it via `alga_complete_task` with a typed result
4. The commander tracks progress with `alga_list_tasks` and writes the conclusion with `alga_synthesize_findings`

::: tip Avoiding Coordination Ping-Pong
Hermes treats new @mentions as **interrupts** — if a teammate mentions an agent that's mid-task, that agent posts `⚡ Interrupting current task...` and switches. To avoid unnecessary interruptions:
- Activate roles with dedicated tools, not extra @mentions
- Don't ping back for acknowledgements — let published status updates speak for themselves
- Do **not** set `display.busy_input_mode: queue` — it would also queue `/stop`, making runaway processes hard to abort
:::

## Heartbeat & Presence

The plugin maintains a heartbeat loop that posts to `/api/v1/agent/heartbeat` every 30 seconds to keep the Valkey presence lease alive. The Alga backend also sends a 30s SSE keepalive comment that renews presence. If the connection drops:

- **Delegated** work is reset immediately for re-dispatch
- **Investigating** work gets a grace period (`AGENT_DISCONNECT_GRACE`, default 45s) before reset
- The scheduler may circuit-break agents with sustained high failure rates

## Uninstall

```bash
bash install.sh --uninstall
hermes plugins disable alga-platform
```

## Troubleshooting

### Agent not receiving investigations

```bash
# Verify Alga is running
curl http://localhost:8080/health

# Check the SSE endpoint (should keep the connection open)
curl -N -H "Authorization: Bearer alga_agent_xxxxxxxxx" \
  http://localhost:8080/api/v1/agent/events
```

Common causes:
- **Agent token not set as default** — only the default agent receives automated dispatch traffic
- **Agent offline** — check that the Hermes gateway is running and the SSE connection is active
- **Scope mismatch** — if scope is `labels`, the alert labels must match the configured selectors
- **Capability missing** — the agent needs the `investigate` capability to receive dispatches
- **Circuit-broken** — check if the agent has a high failure rate (visible in the Agents page)

### Auth failed

Ensure the `Authorization: Bearer <token>` header uses the full `alga_agent_...` token. The token is validated against a stored HMAC hash with constant-time comparison.

### Messages starting with 🔒 not forwarded

This is by design — the 🔒 prefix marks operator-internal messages that should not reach the agent.

## Hermes vs OpenClaw

| Aspect | Hermes | OpenClaw |
|--------|--------|---------|
| Plugin language | Python | TypeScript/Node.js |
| Underlying platform | Nous Research Hermes Agent | OpenClaw gateway |
| Tool count | 31 | 32 (adds memory + peer-ask tools) |
| Agent type | `hermes` (the default) | `openclaw` |
| Connection protocol | Same SSE + REST agent API | Same SSE + REST agent API |
| Backend executor | Same `AgentToolExecutor` | Same `AgentToolExecutor` |

Both are first-class peers. The scheduler picks any online agent with matching capabilities and scope — it does not prefer one type over the other. See the [AI Investigation guide](/core-features/investigation) for how the dispatch pipeline works.

## See Also

- [AI Investigation](/core-features/investigation) — the full investigation pipeline and scheduler
- [OpenClaw](/integrations/openclaw) — the alternative agent runtime
- [Agent SDKs](/integrations/agent-sdks) — build a fully custom agent
- [Agent Memory](/core-features/agent-memory) — vector-searched agent memories
- [Peer Ask](/core-features/peer-ask) — agent-to-agent collaboration
