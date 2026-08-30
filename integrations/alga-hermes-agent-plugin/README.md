# Alga Platform Adapter for Hermes Agent

Self-contained plugin that registers a platform adapter and 31 agent tools for [Hermes Agent](https://github.com/nousresearch/hermes-agent) to use Alga alert and incident threads as chat sessions over SSE + REST.

## Owner-Scoped Work

Alga dispatches work with owner-scoped chat IDs:

- `alert_42` for alert number 42
- `incident_coord_12` for incident coordination thread for incident 12
- `incident_inv_12` for incident investigation working thread for incident 12

The plugin posts investigation progress to owner threads.
Standalone `/api/v1/investigations/*` routes are no longer supported.

## Prerequisites

- Hermes Agent installed (`~/.hermes/` exists)
- `httpx>=0.27` (`pip install 'httpx>=0.27'`)

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            Alga Backend                                 │
│  /api/v1/agent/events ◀── SSE (server push: chat, investigations)      │
│  /api/v1/agent/messages ◀── REST POST (agent → thread)                 │
│  /api/v1/agent/messages/{id} ◀── REST PUT (edit message)               │
│  /api/v1/agent/heartbeat ◀── REST POST (presence keep-alive)           │
│                                                                         │
│  Owner-Scoped Thread                                                   │
│  - User posts message ──▶ Forwarded to Hermes via SSE                  │
│  - Hermes responds ──▶ Posted to thread via REST                        │
└─────────────────────────────────────────────────────────────────────────┘
                                     ▲
                                     │ SSE + REST (httpx)
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         Hermes Gateway                                  │
│  ~/.hermes/plugins/alga-platform/                                       │
│  - Platform adapter (SSE + REST bridge)                                 │
│  - 31 agent tools (inv_tool + REST queries)                            │
│  - chat_id = "alert_42", "incident_coord_12", or "incident_inv_12"      │
└─────────────────────────────────────────────────────────────────────────┘
```

## Installation

```bash
# 1. Install the plugin
cd integrations/alga-hermes-agent-plugin
bash install.sh

# 2. Enable the plugin
hermes plugins enable alga-platform

# 3. Set credentials
cat >> ~/.hermes/.env <<EOF
ALGA_SERVER_URL=http://alga:8080
ALGA_AGENT_TOKEN=alga_agent_xxxxxxxxxxxxxxxx
EOF

# 4. Enable the toolset
hermes tools enable alga

# 5. Restart gateway
hermes gateway restart
```

Uninstall:

```bash
bash install.sh --uninstall
hermes plugins disable alga-platform
```

### What the plugin provides

| Component            | What it does                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Platform adapter** | SSE listener + REST sender for owner-scoped threads                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| **27 agent tools**   | `alga_resolve_alert`, `alga_reopen_alert`, `alga_promote_to_incident`, `alga_set_outcome`, `alga_cancel_investigation`, `alga_pause_investigation`, `alga_search_knowledge`, `alga_get_knowledge`, `alga_create_knowledge`, `alga_list_alerts`, `alga_triage_feedback`, `alga_set_incident_priority`, `alga_set_incident_severity`, `alga_trigger_escalation`, `alga_mitigate_incident`, `alga_resolve_incident`, `alga_set_incident_resolution_docs`, `alga_begin_triage`, `alga_promote_incident`, `alga_add_incident_timeline`, `alga_assign_incident_role`, `alga_get_incident_context`, `alga_get_incident_timeline`, `alga_post_handoff`, `alga_publish_status_update`, `alga_list_services`, `alga_who_is_on_call` |
| **Setup wizard**     | `hermes gateway setup` → choose Alga → prompts for URL + token                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| **Platform hint**    | Injected into the LLM system prompt with Alga-specific guidance                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |

### Interactive setup

The plugin integrates with `hermes gateway setup` — select "Alga" from the platform list and it will prompt for the server URL and agent token.

## Configuration

### Alga Setup

1. Create an **integration agent** (bot token) in the Alga console under **Integrations → Agents → Add agent**.
   Choose **Hermes Agent** as the agent type. Save the bot token when shown; it is not displayed again.
   Mark this agent as **Set as default** if it should receive automated investigation traffic.

### Hermes Agent Setup

Add to `~/.hermes/.env`:

```bash
ALGA_SERVER_URL=http://alga:8080
ALGA_AGENT_TOKEN=alga_agent_xxxxxxxxxxxxxxxx
```

The adapter uses `ALGA_SERVER_URL` as the HTTP(S) origin for both SSE and REST endpoints:

- SSE stream: `GET /api/v1/agent/events?token=<token>`
- REST calls: `POST /api/v1/agent/messages`, `PUT /api/v1/agent/messages/{id}`, `POST /api/v1/agent/heartbeat`

Or in `~/.hermes/config.yaml`:

```yaml
platforms:
  alga:
    enabled: true
    extra:
      server_url: "http://alga:8080"
    token: "${ALGA_AGENT_TOKEN}"
```

#### Private operator chat (Alga UI DM)

```bash
ALGA_HOME_CHANNEL=alga_dm
```

## Message Format

### Alga → Hermes (SSE event)

```json
{
  "type": "message",
  "chat_id": "alert_42",
  "text": "What's the status of the alert?",
  "sender_id": "user_id_abc",
  "sender_name": "John Doe"
}
```

### Hermes → Alga (REST POST)

```json
{
  "chat_id": "alert_42",
  "text": "I'm investigating the high CPU alert now."
}
```

## Reply Messages

The adapter supports Telegram/Discord-style message replies on both directions, matching the Hermes `MessageEvent.reply_to_message_id` / `reply_to_text` contract used by every bundled platform adapter.

### Inbound (Alga → Hermes)

When an Alga thread message is a reply to an earlier message, the SSE `message` event includes the reply context:

```json
{
  "type": "message",
  "chat_id": "alert_42",
  "text": "That was the deploy, not the config.",
  "message_id": "m-2",
  "sender_id": "user_id_abc",
  "sender_name": "John Doe",
  "reply_to_message_id": "m-1",
  "reply_to_text": "Was it the config change?"
}
```

The adapter populates `MessageEvent.reply_to_message_id` / `reply_to_text`, and the Hermes gateway injects a `[Replying to: "..."]` prefix into the agent prompt so the agent knows which prior message the user is referencing.

### Outbound (Hermes → Alga)

When the agent replies, Hermes passes the triggering message id as `reply_to` to `send()`. The adapter forwards it as `reply_to_message_id` in the `POST /api/v1/agent/messages` body:

```json
{
  "chat_id": "alert_42",
  "kind": "text",
  "text": "Confirmed — it was the deploy.",
  "reply_to_message_id": "m-2"
}
```

Only the first chunk of a multi-chunk message carries the reply-to id. The field maps to the `reply_to_message_id` column on `investigation_thread_messages`; the backend persists and renders the reply context.

## Internal Messages

Messages in Alga threads starting with 🔒 are **NOT** forwarded to Hermes:

```
🔒 Let's discuss offline first    → Not forwarded
What's the status?                  → Forwarded to Hermes
```

## Slash Commands

Typing `/stop` in an Alga thread cancels the agent's in-progress generation for that thread. The command is forwarded to Hermes even when the agent was not explicitly mentioned.

No other slash commands are special-cased; all other observe-triggered messages are appended to the transcript without interrupting the agent.

## Investigation Commands

Agent messages posted through the adapter are sent to Alga verbatim and land in the owner thread as prose. Alert-lifecycle and investigation-metadata mutations must be performed by calling the **unified** `POST /api/v1/agent/messages` endpoint with `kind: "inv_tool"`; Alga does not parse slash commands out of chat text.

### REST endpoint

```
POST {ALGA_SERVER_URL}/api/v1/agent/messages
Authorization: Bearer <agent token>
Content-Type: application/json
```

Request body:

```json
{
  "chat_id": "alert_42",
  "kind": "inv_tool",
  "command": {
    "op": "set_outcome",
    "root_cause": "Memory leak in worker process causing OOM kills",
    "resolution": "Restarted worker pods and applied memory limit patch"
  }
}
```

`chat_id` is the owner-scoped chat ID (e.g. `alert_42`, `incident_coord_12`, `incident_inv_12`). `kind` is **required** (`"text"` or `"inv_tool"`); omitting it returns HTTP 400.

### Supported ops

All op fields live under `command.*`.

| `command.op`                   | Required fields                                                                                                                         | Optional fields                                                                                                                       | Description                                                                                                                      |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `resolve_alert`                | —                                                                                                                                       | `command.fingerprint`                                                                                                                 | Resolve an alert                                                                                                                 |
| `reopen_alert`                 | —                                                                                                                                       | `command.fingerprint`                                                                                                                 | Reopen a resolved alert                                                                                                          |
| `set_outcome`                  | at least one of `command.root_cause` / `command.resolution`                                                                             | —                                                                                                                                     | Record the root cause and/or resolution                                                                                          |
| `cancel_investigation`         | `command.reason`                                                                                                                        | —                                                                                                                                     | Cancel the investigation                                                                                                         |
| `pause_investigation`          | `command.reason`                                                                                                                        | —                                                                                                                                     | Pause the investigation                                                                                                          |
| `set_incident_severity`        | `command.severity` (`critical`\|`high`\|`warning`\|`info`)                                                                              | —                                                                                                                                     | Set incident severity                                                                                                            |
| `begin_triage`                 | —                                                                                                                                       | —                                                                                                                                     | Move incident to 'triaging' status                                                                                               |
| `promote_incident`             | —                                                                                                                                       | —                                                                                                                                     | Promote incident from 'triaging' to 'active' status                                                                              |
| `assign_incident_role`         | `command.role_type`                                                                                                                     | `command.user_id`, `command.agent_token_id`, `command.scope_description`                                                              | Assign ICS role to user/agent                                                                                                    |
| `resolve_incident`             | —                                                                                                                                       | `command.reason`, `command.summary`, `command.impact_assessment`, `command.actions_taken`, `command.root_cause`, `command.resolution` | Resolve incident; the resolution fields are recorded when supplied and are mandatory before resolution succeeds (commander only) |
| `set_incident_resolution_docs` | at least one of `command.summary` / `command.impact_assessment` / `command.actions_taken` / `command.root_cause` / `command.resolution` | —                                                                                                                                     | Record resolution documents without resolving (commander only)                                                                   |

### Incident role boundaries

The Alga backend enforces incident role boundaries for Hermes tokens. The plugin descriptions mirror these rules so the model does not keep retrying tools outside its active role:

Coordination is **message-driven**. The commander delegates technical work by @mentioning responder agents in the coordination thread (a mention activates the agent) and by assigning child incident investigations; responder handoffs flow back through `alga_post_handoff` with a structured result. The commander tracks progress through the coordination thread, investigation summaries, and the Status Updates card, and writes the incident conclusion itself. The legacy `alga_request_status_update` flow and the removed coordination-task subsystem (`alga_dispatch_task`, `alga_claim_task`, `alga_complete_task`, `alga_list_tasks`, `alga_synthesize_findings`) are gone.

| Active incident role               | Allowed incident actions                                                                                                                               |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Incident commander                 | Priority, escalation, mitigation, resolution, resolution documents, triage/promote, role assignment, delegation via @mentions and child investigations |
| Responder or assigned investigator | Investigation thread updates, severity, outcome, pause/cancel investigation, milestone status updates, structured handoff via `alga_post_handoff`      |
| Communications lead                | Publishing public status updates on request (mention or `comms_stale` nudge)                                                                           |

> Resolution requires five structured artifacts — `summary`, `impact_assessment`, `actions_taken`, `root_cause`, and `resolution`. The `root_cause` and `resolution` sections are incident document cards shown on the incident page and are independently mandatory (actions_taken no longer satisfies resolution). A commander supplies them inline to `resolve_incident` or stages them with `set_incident_resolution_docs`, then resolves. To ask the communicator to publish a status update, the commander @mentions them in the coordination thread. `alga_get_incident_context`, `alga_list_alerts`, and `alga_add_incident_timeline` require the `investigate` capability and are denied to a pure commander token — ask the responder for that information instead.

If Alga returns `403` or an `inv_tool` error saying the agent is not assigned or not authorized, the Hermes agent should stop using that tool for the incident and send a coordination update to the appropriate role instead.

### Multi-agent incident coordination

During an incident, several Hermes agents (commander, communicator, responder) share the incident coordination thread and @mention each other. By default Hermes treats a new mention as an **interrupt**: if a teammate mentions an agent that is mid-task, that agent posts `⚡ Interrupting current task...` and switches to the new message. This is intentional — it keeps `/stop` and operator interjection working against long-running or runaway turns.

Because of that, the goal is not to silence interruptions (e.g. do **not** set Hermes `display.busy_input_mode: queue` for Alga agents — it would also queue `/stop` and make runaway processes hard to abort). The goal is to avoid _unnecessary_ ones:

- Activate a role only when needed. A commander asks the communicator for a status update by @mentioning them in the coordination thread with the exact level and content request; a mention wakes the agent, so avoid redundant follow-up mentions. A responder publishes status directly with `alga_publish_status_update` rather than delegating to the communicator.
- Don't ping back for acknowledgements. After publishing a status update, the communicator should let the published update speak for itself (the commander polls the timeline / status updates) rather than @mentioning the commander back. Likewise the commander checks the timeline instead of re-requesting.
- Status updates are a comms log and remain repeatable at every level (including `resolved`); the backend never gates them by `status_level`.

This keeps the coordination thread responsive and `/stop`-able while minimizing cross-talk between agents.

### How It Works

1. The investigation prompt (built by the Alga backend) documents the available ops and their payloads.
2. The agent posts its analysis as normal messages via `send()` / `edit_message()` (body `{chat_id, kind: "text", text}`); those are stored as thread comments and forwarded to Mattermost/Slack unchanged.
3. Separately, for each lifecycle/metadata mutation the agent calls `POST /api/v1/agent/messages` with `kind: "inv_tool"` and a `command` block (shape above).
4. Alga executes the mutation and posts a human-readable confirmation (e.g., "✅ **Hermes** resolved alert #1 HighCPU") to the owner thread.

## Session ID Mapping

| Alga Concept                            | Hermes Concept                                        |
| --------------------------------------- | ----------------------------------------------------- |
| `alert_42`                              | `chat_id = "alert_42"`                                |
| `incident_coord_12` / `incident_inv_12` | `chat_id = "incident_coord_12"` / `"incident_inv_12"` |
| User reply                              | `MessageEvent.text`                                   |

## Agent Tools

The plugin registers 31 tools under the `"alga"` toolset via `ctx.register_tool()`.

| Tool                                | Description                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `alga_resolve_alert`                | Resolve alert and close investigation. Optionally record root cause and resolution.                                                                                                                                                                                                                                                                                                                                                            |
| `alga_reopen_alert`                 | Reopen a previously resolved alert and resume investigation.                                                                                                                                                                                                                                                                                                                                                                                   |
| `alga_promote_to_incident`          | Promote an alert to an incident from alert investigation. Borrow the investigation summary as the incident description.                                                                                                                                                                                                                                                                                                                        |
| `alga_set_outcome`                  | Record root cause and/or resolution without resolving the alert.                                                                                                                                                                                                                                                                                                                                                                               |
| `alga_cancel_investigation`         | Cancel the current investigation with a reason.                                                                                                                                                                                                                                                                                                                                                                                                |
| `alga_pause_investigation`          | Pause investigation when waiting for external events.                                                                                                                                                                                                                                                                                                                                                                                          |
| `alga_search_knowledge`             | Search Alga knowledge notes (runbooks, known issues, service owner docs, facts). Returns short previews with each note's id.                                                                                                                                                                                                                                                                                                                   |
| `alga_get_knowledge`                | Fetch the full body of a single knowledge note by id (search only returns a 200-char preview).                                                                                                                                                                                                                                                                                                                                                 |
| `alga_create_knowledge`             | Create a knowledge note from investigation findings.                                                                                                                                                                                                                                                                                                                                                                                           |
| `alga_list_alerts`                  | Query alerts beyond the current investigation's primary alert.                                                                                                                                                                                                                                                                                                                                                                                 |
| `alga_triage_feedback`              | Provide feedback on a triage decision.                                                                                                                                                                                                                                                                                                                                                                                                         |
| `alga_set_incident_priority`        | Set incident priority (P1-P5).                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `alga_set_incident_severity`        | Set incident severity (critical, high, warning, info).                                                                                                                                                                                                                                                                                                                                                                                         |
| `alga_trigger_escalation`           | Trigger escalation for an incident.                                                                                                                                                                                                                                                                                                                                                                                                            |
| `alga_mitigate_incident`            | Mark an incident as mitigated.                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `alga_resolve_incident`             | Resolve an incident. Accepts inline `summary`, `impact_assessment`, `actions_taken`, `root_cause`, `resolution` (recorded before resolving). `root_cause` and `resolution` are mandatory document sections. Blocked until a communicator has published a status update (when one is assigned).                                                                                                                                                 |
| `alga_set_incident_resolution_docs` | Commander-only. Record the `summary`, `impact_assessment`, `actions_taken`, `root_cause`, `resolution` resolution documents without resolving (required before an incident can be resolved).                                                                                                                                                                                                                                                   |
| `alga_begin_triage`                 | Move incident to 'triaging' status.                                                                                                                                                                                                                                                                                                                                                                                                            |
| `alga_promote_incident`             | Promote incident to 'active' status.                                                                                                                                                                                                                                                                                                                                                                                                           |
| `alga_add_incident_timeline`        | Log a custom note or status update to the incident timeline.                                                                                                                                                                                                                                                                                                                                                                                   |
| `alga_assign_incident_role`         | Assign ICS command roles (e.g., commander, comms lead) to users/agents.                                                                                                                                                                                                                                                                                                                                                                        |
| `alga_get_incident_context`         | Get full incident context including status, severity, timeline, roles.                                                                                                                                                                                                                                                                                                                                                                         |
| `alga_get_incident_timeline`        | Get the timeline of events for an incident.                                                                                                                                                                                                                                                                                                                                                                                                    |
| `alga_post_handoff`                 | Commander-facing coordination update for the **final handoff only**. WARNING: every call activates other agents (commander, communicator) and can interrupt their current work, causing ping-pong loops. Responders are forbidden from calling this during investigation/identification/mitigation/verification — use `alga_publish_status_update` for status milestones instead. Set `audience` to direct it at a role.                       |
| `alga_publish_status_update`        | Publish a public-facing status update with a `status_level` (investigating, identified, monitoring, resolved) — does NOT activate other agents, so it is the preferred tool for status communication during active work. Available to commander, responder, and communicator. Responders may only publish `identified` or `monitoring` (never `resolved` or `investigating`). At least one must be published before the commander can resolve. |
| `alga_list_services`                | List all registered services with their current status.                                                                                                                                                                                                                                                                                                                                                                                        |
| `alga_who_is_on_call`               | Get the current on-call person for each schedule.                                                                                                                                                                                                                                                                                                                                                                                              |

All tools are gated by `_check_tool_availability()` (requires `ALGA_SERVER_URL` + `ALGA_AGENT_TOKEN`). The tools make direct REST calls to the Alga backend using `httpx`. Investigation lifecycle tools (`resolve`, `reopen`, `set_outcome`, `cancel`, `pause`) and incident mutation tools send `inv_tool` payloads via `POST /api/v1/agent/messages`. Knowledge and query tools call their respective `GET /api/v1/agent/*` endpoints.

## Dependencies

```bash
pip install httpx>=0.27
```

## Compatibility

The plugin uses only the **official Hermes plugin API**:

| API surface                               | Used for                      | Stability                                                 |
| ----------------------------------------- | ----------------------------- | --------------------------------------------------------- |
| `plugin.yaml` manifest (`kind: platform`) | Plugin discovery              | Stable — same format as bundled IRC/Teams plugins         |
| `PluginContext.register_platform()`       | Platform adapter registration | Stable — official plugin entry point                      |
| `PluginContext.register_tool()`           | Tool registration             | Stable — used by all tool-providing plugins               |
| `BasePlatformAdapter` interface           | SSE/REST adapter              | **Internal** — may change across major versions           |
| `PlatformConfig` / `Platform("alga")`     | Config and enum               | **Internal** — `_missing_()` creates dynamic members      |
| `PlatformEntry` fields                    | Platform metadata             | Stable — `allowed_users_env`, `platform_hint`, `setup_fn` |

**Risk areas for future Hermes releases:**

- `BasePlatformAdapter` method signatures (`connect`, `disconnect`, `send`, `edit_message`, `build_source`, etc.) could gain required parameters. The bundled IRC/Teams adapters use the same interface, so this is unlikely to break silently.
- `_set_fatal_error`, `_mark_connected`, `_mark_disconnected`, `_notify_fatal_error` are helper methods on the base class — not part of the documented adapter contract but used by all built-in adapters.
- If Hermes drops `_missing_()` dynamic enum members, the plugin would need a one-line enum addition.

**Mitigation:** The IRC and Teams platform plugins (`plugins/platforms/irc/`, `plugins/platforms/teams/`) are bundled with Hermes and use the exact same pattern. As long as those plugins continue to work, the Alga plugin will too.

## Testing

1. Start Alga: `cd apps/backend && go run .`
2. Create a default integration agent in Alga and configure Hermes with that token
3. Start Hermes gateway: `hermes gateway`
4. Trigger an alert in Alga to create an investigation
5. Reply to the investigation thread
6. Hermes should receive the message via SSE and respond via REST

## Troubleshooting

```bash
# Check Alga is running
curl http://localhost:8080/health

# Check SSE endpoint (should keep connection open)
curl -N -H "Authorization: Bearer alga_agent_xxxxxxxxx" \
  http://localhost:8080/api/v1/agent/events
```

- No messages: check agent token is valid and set as default, investigation exists, messages don't start with 🔒
- Auth failed: ensure `Authorization: Bearer <default agent bot token>`

## License

Same as parent Alga project.
