---
title: OpenClaw Agent
description: Connect an OpenClaw agent gateway to Alga for autonomous AI investigation with 32 alga_* tools, built-in memory, peer-ask collaboration, and multi-account support.
---

# OpenClaw Agent

[OpenClaw](https://openclaw.ai) is an autonomous AI agent gateway that connects to Alga via a self-contained TypeScript channel plugin, allowing OpenClaw-powered agents to act as AI SRE investigators — receiving alert dispatches over SSE, reasoning about root causes, and taking lifecycle actions (resolve, escalate, promote to incident) through 32 `alga_*` tools.

OpenClaw and [Hermes](/agents/hermes) are **alternative, peer agent runtimes** — both plug into the same Alga agent API. You can run either one, or both as different agent tokens. The scheduler is agent-type-agnostic: any online agent with the right capabilities and scope can win a dispatch.

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│                        Alga Backend                          │
│  /api/v1/agent/events ◀── SSE (dispatch, chat, lifecycle)   │
│  /api/v1/agent/messages ◀── REST POST (agent → thread)      │
│  /api/v1/agent/heartbeat ◀── REST POST (presence)           │
└─────────────────────────────────────────────────────────────┘
                              ▲
                              │ SSE + REST (eventsource2 + fetch)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     OpenClaw Gateway                          │
│  ~/.openclaw/extensions/alga/                                 │
│  - Channel plugin (SSE + REST bridge)                         │
│  - 32 agent tools under the "alga" toolset                    │
│  - chat_id = "alert_42", "incident_coord_12", etc.           │
└─────────────────────────────────────────────────────────────┘
```

**The flow:** Alert → Correlator → Scheduler → OpenClaw agent (SSE dispatch) → agent reasons and calls `alga_*` tools → Alga executes mutations and posts confirmations to the investigation thread → Findings forwarded to Mattermost/Slack.

The plugin maintains agent presence by sending a heartbeat (`POST /api/v1/agent/heartbeat`) every 30 seconds, which is how Alga marks the agent as online and eligible to receive dispatches. If the SSE connection drops, the plugin reconnects automatically after 5 seconds.

## Prerequisites

- [OpenClaw](https://openclaw.ai) v2026.6.10+ installed and running
- Node.js 22+ (for the standalone installer)
- A reachable Alga backend instance

## Setup Guide

### Step 1: Create an Agent Token in Alga

1. In the Alga web UI, go to **Agents → Add agent**
2. Choose **OpenClaw** as the agent type
3. Select the capabilities you need (at minimum: `investigate`)
4. Set the scope (`all` for catch-all, or `labels` to restrict to specific alert labels)
5. **Check "Set as default"** if this agent should receive automated investigation traffic when no label-targeted agent matches
6. Save — **copy the token immediately**, it's shown only once (`alga_agent_...`)

::: tip
The `openclaw` agent type is recognized explicitly by Alga. If you leave the type unspecified or enter an unrecognized value, it normalizes to `hermes` (the default).
:::

### Step 2: Install the Plugin

Using the OpenClaw CLI:

```bash
# From a local checkout
openclaw plugins install /path/to/alga/integrations/alga-openclaw-plugin

# Or from npm (once published)
openclaw plugins install npm:@alga/openclaw-plugin

# Local development with symlink
openclaw plugins install -l ./integrations/alga-openclaw-plugin
```

Alternatively, use the standalone installer (no OpenClaw CLI required):

```bash
cd integrations/alga-openclaw-plugin
bash install.sh \
  --server-url http://alga:8080 \
  --token alga_agent_xxxxxxxxxxxxxxxx
```

The installer copies the plugin into `~/.openclaw/extensions/alga/`, installs runtime dependencies, and merges all 32 tool names into `tools.alsoAllow` in your OpenClaw config.

### Step 3: Configure Credentials

Add to your environment:

```bash
export ALGA_SERVER_URL=http://alga:8080
export ALGA_AGENT_TOKEN=alga_agent_xxxxxxxxxxxxxxxx
```

Or in `~/.openclaw/openclaw.json`:

```json
{
  "channels": {
    "alga": {
      "serverUrl": "http://alga:8080",
      "token": "alga_agent_xxxxxxxxxxxxxxxx"
    }
  }
}
```

Config values take precedence over environment variables when both are set.

### Step 4: Restart the Gateway

```bash
openclaw gateway restart
```

On startup, the plugin automatically verifies that all 32 `alga_*` tools are in your `tools.alsoAllow` list and merges any missing ones.

### Step 5: Verify

Create a test notification from your Grafana or webhook source, or use Alga's **Create Alert** button. The OpenClaw agent should appear online (**Agents** shows a green dot) and begin working the investigation.

### Interactive Setup (Alternative)

The plugin integrates with the OpenClaw setup wizard — run `openclaw setup` and select **"Alga (investigations)"** from the channel list. It will prompt for the server URL and agent token interactively, with automatic detection if both are already set as environment variables.

## Configuration Reference

### Environment Variables

| Variable             | Required | Description                                                                                                |
| -------------------- | -------- | ---------------------------------------------------------------------------------------------------------- |
| `ALGA_SERVER_URL`    | Yes      | Alga backend base URL (no trailing slash), e.g. `http://alga:8080`. Also accepts `ALGA_URL` as a fallback. |
| `ALGA_AGENT_TOKEN`   | Yes      | The `alga_agent_...` bearer token from Alga                                                                |
| `ALGA_ALLOWED_USERS` | No       | Comma-separated OpenClaw user IDs allowed to use the channel (default: allow all)                          |

### Channel Config (`channels.alga`)

| Key              | Type                 | Default                          | Description                                                                          |
| ---------------- | -------------------- | -------------------------------- | ------------------------------------------------------------------------------------ |
| `name`           | string               | —                                | Account display name                                                                 |
| `enabled`        | boolean              | `true`                           | Whether the channel is active (disabled only when explicitly `false`)                |
| `serverUrl`      | string               | falls back to `ALGA_SERVER_URL`  | Alga backend base URL (no trailing slash)                                            |
| `token`          | string               | falls back to `ALGA_AGENT_TOKEN` | Bot token from the Alga UI (`alga_agent_...`)                                        |
| `allowFrom`      | `(string\|number)[]` | `["*"]` (allow all)              | OpenClaw user IDs allowed to use the Alga channel                                    |
| `defaultTo`      | string               | —                                | Default conversation target (e.g. `alga_dm` for operator private chat)               |
| `accounts`       | object               | —                                | Multi-account support — map of accountId → partial account config with the same keys |
| `defaultAccount` | string               | —                                | Which account ID is the default                                                      |

## Owner-Scoped Threads

Alga dispatches work using owner-scoped chat IDs that the plugin maps to OpenClaw agent sessions:

| Chat ID Pattern     | Meaning                                       |
| ------------------- | --------------------------------------------- |
| `alert_42`          | Alert investigation thread for alert #42      |
| `incident_coord_12` | Incident coordination thread for incident #12 |
| `incident_inv_12`   | Incident investigation working thread         |
| `alga_dm`           | Private operator DM chat                      |

Bare numbers passed as an `investigation_id` argument are treated as alert numbers. When an incident tool is called with a bare number, the plugin automatically builds the `incident_coord_<number>` chat ID.

## Message Flow

### Alga → OpenClaw (SSE dispatch)

When an alert is dispatched, the agent receives an SSE `message` event containing the full investigation prompt — alert details, labels, annotations, shared knowledge, triage enrichment, playbook steps, and role-specific instructions.

```json
{
  "type": "message",
  "chat_id": "alert_42",
  "text": "What's the status of the alert?",
  "sender_id": "user_id_abc",
  "sender_name": "John Doe"
}
```

### OpenClaw → Alga (REST)

The agent responds by posting messages and executing tools via REST. Each reasoning segment is posted as its own message before the following tool call, and tool calls are narrated as `🧩 tool_name [key arg]` on their own line so operators can follow along in real time.

```json
{
  "chat_id": "alert_42",
  "text": "I'm investigating the high CPU alert now."
}
```

### Thread replies

Both directions support Telegram/Discord-style message replies. When a thread message is a reply to an earlier message, the SSE event includes `reply_to_message_id` and `reply_to_text`, and the agent receives the replied-to context.

### Internal messages

Messages starting with 🔒 are **not forwarded** to the agent — use this prefix for operator-only context that shouldn't reach the agent.

### Slash commands

- `/stop` — cancels the agent's in-progress generation for that thread and suppresses further outbound messages until a new inbound message arrives
- All other messages are appended to the transcript as observation context

## The 32 Agent Tools

The plugin registers 32 tools. Each tool either sends an `inv_tool` command via `POST /api/v1/agent/messages` or makes a direct REST query to a dedicated agent endpoint.

### Alert & Investigation Tools

| Tool                        | Description                                                                                                                                                 |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `alga_resolve_alert`        | Resolve an alert and close the investigation (optionally record root cause + resolution). Investigation completes only when ALL linked alerts are resolved. |
| `alga_reopen_alert`         | Reopen a resolved alert. Re-activates a completed investigation and re-delegates to an agent.                                                               |
| `alga_promote_to_incident`  | Promote an alert investigation to a full incident (borrows the investigation summary as the incident description).                                          |
| `alga_set_outcome`          | Record root cause and/or resolution without resolving — use for progressive documentation during investigation.                                             |
| `alga_cancel_investigation` | Cancel the investigation (false positive, transient, not actionable). Requires a reason.                                                                    |
| `alga_pause_investigation`  | Pause investigation when waiting for external events or human input. Requires a reason.                                                                     |
| `alga_list_alerts`          | Query the broader alert landscape (filter by status, severity, search text).                                                                                |
| `alga_triage_feedback`      | Confirm or override a triage decision to improve future accuracy.                                                                                           |
| `alga_assign_investigation` | Reassign the current investigation to a different agent. Only the currently assigned agent can reassign.                                                    |

### Knowledge Tools

| Tool                    | Description                                                                                                      |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `alga_search_knowledge` | Search shared knowledge notes (runbooks, known issues, service owners, facts) — returns 200-char previews.       |
| `alga_get_knowledge`    | Fetch the full body of a knowledge note by ID.                                                                   |
| `alga_create_knowledge` | Create a reusable knowledge note from investigation findings (kinds: runbook, known_issue, service_owner, fact). |

### Memory Tools (OpenClaw-exclusive)

These tools are unique to OpenClaw — Hermes does not expose them. They give the agent its own private recall across investigations.

| Tool                   | Description                                                                                                        |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `alga_search_memories` | Search agent-scoped memories from past investigations (semantic vector search). Useful for finding past solutions. |
| `alga_create_memory`   | Create an agent memory for future investigations — persist useful findings, fixes, and insights.                   |

::: tip Memory vs Knowledge — what's the difference?
`alga_search_knowledge` queries the **shared, operator-curated knowledge base** (runbooks, known issues, service owner contacts). `alga_search_memories` queries the **agent's own episodic memories** — things it learned during past investigations and saved for itself. See [Agent Memory](/agents/memory) for the backend system.
:::

### Peer Collaboration Tools (OpenClaw-exclusive)

| Tool            | Description                                                                                                                                                                                                            |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `alga_peer_ask` | Ask another agent for help — useful when you need expertise from a specialized agent. Creates a peer-ask request that the target agent receives over SSE. Discovers online agents of either type (hermes or openclaw). |

See [Peer Ask](/agents/peer-ask) for the full agent-to-agent collaboration protocol, SSE frame types, and limits.

### Incident Command Tools (Commander-Only)

| Tool                                | Description                                                                                                                                       |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `alga_set_incident_priority`        | Set incident priority (P1–P5). Affects SLA targets.                                                                                               |
| `alga_set_incident_severity`        | Set incident severity (critical, high, warning, info).                                                                                            |
| `alga_trigger_escalation`           | Trigger escalation for the incident — notifies on-call responders and escalation contacts.                                                        |
| `alga_request_status_update`        | Request a status update from incident responders — sends a notification asking for a progress report.                                             |
| `alga_mitigate_incident`            | Mark an incident as mitigated (impact contained, root cause may not be fully resolved).                                                           |
| `alga_resolve_incident`             | Resolve an incident (root cause addressed, fully closed).                                                                                         |
| `alga_set_incident_resolution_docs` | Stage structured resolution documents (summary, impact, actions, root cause, resolution) without resolving.                                       |
| `alga_begin_triage`                 | Move incident from `detected` to `triaging`.                                                                                                      |
| `alga_promote_incident`             | Promote incident from `triaging` to `active`.                                                                                                     |
| `alga_assign_incident_role`         | Assign ICS command roles (incident_commander, operations_lead, communications_lead, scribe, etc.) to users or agents.                             |
| `alga_post_handoff`                 | Commander-facing **final** handoff only — see warning below.                                                                                      |
| `alga_publish_status_update`        | Publish a public status update (`investigating`, `identified`, `monitoring`, `resolved`). The only path that creates a Status Updates card entry. |

::: warning `alga_post_handoff` activates other agents
Every `alga_post_handoff` call **wakes up teammate agents** (commander, communicator) by forwarding the message to them, which can interrupt their current work and cause ping-pong loops. Reserve this tool for the single structured commander handoff that happens **after recovery is verified** and a `monitoring` status update has already been published via `alga_publish_status_update`.

For status milestones during active work (identified, monitoring, resolved), **always use `alga_publish_status_update`** instead — it does not activate other agents.
:::

### Incident Query Tools

| Tool                         | Description                                                                                   |
| ---------------------------- | --------------------------------------------------------------------------------------------- |
| `alga_get_incident_context`  | Get full incident context (status, severity, timeline, roles, linked alerts/investigations).  |
| `alga_get_incident_timeline` | Get the timeline of events for an incident.                                                   |
| `alga_add_incident_timeline` | Log a custom entry to the incident timeline (progress, finding, action, resolution, comment). |

### Utility Tools

| Tool                  | Description                                       |
| --------------------- | ------------------------------------------------- |
| `alga_list_services`  | List all registered services with current status. |
| `alga_who_is_on_call` | Get the current on-call person for each schedule. |

## Incident Role Boundaries

The Alga backend enforces incident role boundaries server-side. The plugin mirrors these rules in its tool descriptions so the model doesn't waste calls on tools outside its role:

| Active Role             | Allowed Actions                                                                                                         |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| **Incident Commander**  | Priority, escalation, mitigation, resolution, resolution docs, triage/promote, role assignment, status updates, handoff |
| **Responder**           | Investigation updates, severity, outcome, pause/cancel, timeline entries                                                |
| **Communications Lead** | Publish public status updates, add timeline entries                                                                     |

::: warning Resolution Requirements
Incident resolution requires five structured artifacts: `summary`, `impact_assessment`, `actions_taken`, `root_cause`, and `resolution`. The `root_cause` and `resolution` sections are independently mandatory. A commander supplies them inline to `resolve_incident` or stages them with `set_incident_resolution_docs` first.
:::

## Agent Behavioral Hints

The plugin injects mandatory behavioral instructions into the agent's message-tool context so operators get consistent, readable output regardless of the underlying model:

1. **You are an SRE investigation agent** — operators are reading your output in real time
2. **Follow backend-provided instructions** — the 'Investigation Instructions' and 'Incident Instructions' are authoritative
3. **Write visible text before and after every tool call** — thinking blocks are invisible to operators
4. **Narrate tool calls** as `🧩 tool_name [key arg]` on their own line
5. **Write a final markdown summary** — alert/fingerprint, status, findings, root cause, evidence, resolution, runbook references
6. **Never output `NO_REPLY` or `HEARTBEAT_OK`** — these are reserved protocol messages
7. **Interleave analysis and tool calls** — batch independent tool calls together
8. **Never use exec/curl to call the Alga API** — use the dedicated tools

## Multi-Account Support

The plugin supports connecting a single OpenClaw gateway to multiple Alga instances via the `accounts` config block. Each account has its own `serverUrl`, `token`, `allowFrom`, and `defaultTo`. Use `defaultAccount` to select which receives traffic when no account is specified.

```json
{
  "channels": {
    "alga": {
      "defaultAccount": "production",
      "accounts": {
        "production": {
          "serverUrl": "https://alga.prod.example.com",
          "token": "alga_agent_prod_token..."
        },
        "staging": {
          "serverUrl": "https://alga.staging.example.com",
          "token": "alga_agent_staging_token..."
        }
      }
    }
  }
}
```

## Installer Reference

The standalone installer (`install.sh` / `install.js`) is idempotent and supports these options:

| Option                   | Description                                                            |
| ------------------------ | ---------------------------------------------------------------------- |
| `--profile <name>`       | Install to a named OpenClaw profile (`~/.openclaw-<name>`)             |
| `--server-url <url>`     | Alga backend base URL (env: `ALGA_SERVER_URL`)                         |
| `--token <token>`        | Alga agent token (env: `ALGA_AGENT_TOKEN`)                             |
| `--allowed-users <list>` | Comma-separated OpenClaw user IDs (env: `ALGA_ALLOWED_USERS`)          |
| `--link`                 | Symlink source into extension dir instead of copying (for development) |
| `--skip-build`           | Skip `npm install --omit=dev` when `node_modules` is missing           |
| `--status`               | Check installation status and exit                                     |
| `--uninstall`            | Remove plugin files and config entries                                 |

```bash
# Check if the plugin is installed correctly
bash install.sh --status

# Uninstall
bash install.sh --uninstall
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

- **Agent not online** — verify the OpenClaw gateway is running and the SSE connection is active (**Agents** should show a green dot)
- **Agent token not set as default** — only the default agent receives automated dispatch traffic
- **Tools not in allowlist** — ensure all 32 `alga_*` tools are in `tools.alsoAllow` (the plugin auto-merges on startup, but a restart may be needed)
- **Scope mismatch** — if scope is `labels`, the alert labels must match the configured selectors
- **Capability missing** — the agent needs the `investigate` capability to receive dispatches
- **Wrong server URL** — verify `ALGA_SERVER_URL` / `channels.alga.serverUrl` has no trailing slash and is reachable from the OpenClaw host

### Agent says tools aren't available

Ensure `tools.alsoAllow` in `~/.openclaw/openclaw.json` includes all `alga_*` tool names. The plugin auto-configures this on startup, but if the gateway was already running during installation, restart it:

```bash
openclaw gateway restart
```

### Auth failed

Ensure the `Authorization: Bearer <token>` header uses the full `alga_agent_...` token. The token is validated against a stored HMAC hash with constant-time comparison.

### Investigation completes but alert isn't resolved

The agent must explicitly call `alga_resolve_alert` to resolve alerts. Text messages alone don't resolve alerts — they only document findings. Check the investigation thread for tool call results.

### Messages starting with 🔒 not forwarded

This is by design — the 🔒 prefix marks operator-internal messages that should not reach the agent.

## OpenClaw vs Hermes

| Aspect              | OpenClaw                                                                                                                 | Hermes                                                                                    |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------- |
| Plugin language     | TypeScript/Node.js (ESM)                                                                                                 | Python                                                                                    |
| Underlying platform | OpenClaw gateway                                                                                                         | Nous Research Hermes Agent                                                                |
| Tool count          | 32                                                                                                                       | 31                                                                                        |
| Agent type          | `openclaw`                                                                                                               | `hermes` (the default)                                                                    |
| **Unique tools**    | `alga_search_memories`, `alga_create_memory`, `alga_peer_ask`, `alga_assign_investigation`, `alga_request_status_update` | `alga_dispatch_task`, `alga_complete_task`, `alga_list_tasks`, `alga_synthesize_findings` |
| Coordination model  | `post_handoff` + `publish_status_update` (role-targeted messages)                                                        | Task-driven (`dispatch_task` / `complete_task` / `synthesize_findings`)                   |
| Message delivery    | Per-segment one-shot messages (no draft preview)                                                                         | In-place draft editing via `/api/v1/agent/drafts`                                         |
| Reconnect strategy  | Fixed 5s delay                                                                                                           | Exponential backoff (2s base, 60s max)                                                    |
| Multi-account       | Yes (per-account serverUrl/token)                                                                                        | No (single server per plugin)                                                             |
| Connection protocol | Same SSE + REST agent API                                                                                                | Same SSE + REST agent API                                                                 |
| Backend executor    | Same `AgentToolExecutor`                                                                                                 | Same `AgentToolExecutor`                                                                  |

Both are first-class peers. The scheduler picks any online agent with matching capabilities and scope — it does not prefer one type over the other. See the [AI Investigation guide](/core-features/investigation) for how the dispatch pipeline works.

### When to choose OpenClaw

- You want **agent memory tools** (`alga_search_memories`, `alga_create_memory`) exposed directly to the agent
- You want **peer-ask** capability for agent-to-agent consultation
- You need **multi-account** support (one gateway, multiple Alga instances)
- Your team's expertise is in **TypeScript/Node.js** (easier to extend or debug the plugin)
- You're already running OpenClaw for other agent workloads

### When to choose Hermes

- You want **task-driven coordination** (commander decomposes incidents into typed tasks dispatched to roles)
- You prefer **Python** for plugin customization
- You want **draft streaming** (in-place message editing as the agent reasons)
- You're already running a Hermes gateway

## See Also

- [AI Investigation](/core-features/investigation) — the full investigation pipeline and scheduler
- [Hermes Agent](/agents/hermes) — the alternative agent runtime
- [Agent SDKs](/agents/agent-sdks) — build a fully custom agent
- [Agent Memory](/agents/memory) — vector-searched agent memories
- [Peer Ask](/agents/peer-ask) — agent-to-agent collaboration
