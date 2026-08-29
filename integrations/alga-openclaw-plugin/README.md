# Alga — OpenClaw Channel Plugin

Connect [OpenClaw](https://openclaw.ai) to [Alga](https://github.com/hahnavi/alga) as an SRE investigation agent. The plugin receives investigation dispatches over SSE and reports findings, runs commands, and chats with operators over REST.

## Prerequisites

- Node.js 22+
- OpenClaw gateway v2026.5.7+ (`openclaw` CLI)
- An Alga agent token (from the Alga UI: **Integrations → Agents → Add agent → OpenClaw**)
- The Alga backend must be reachable from the OpenClaw host

## Quick Start

### 1. Create an agent token in Alga

Go to **Integrations → Agents → Add agent**, choose **OpenClaw**, copy the token. Set **Set as default** if this agent should receive investigation dispatches.

### 2. Install the plugin into OpenClaw

```bash
# From a local directory (development)
openclaw plugins install /path/to/eidl2026-alga/integrations/alga-openclaw-plugin

# From npm (once published)
openclaw plugins install npm:@alga/openclaw-plugin
```

### 3. Configure the channel

The plugin setup wizard prompts for server URL and token. Use environment variables for automation:

```sh
export ALGA_SERVER_URL=https://alga.example.com
export ALGA_AGENT_TOKEN=alga_agent_...
```

Or add to `~/.openclaw/openclaw.json`:

```json
{
  "channels": {
    "alga": {
      "serverUrl": "https://alga.example.com",
      "token": "alga_agent_..."
    }
  }
}
```

### 4. Enable the tools

**The plugin auto-configures tool permissions on startup.** If running manually, ensure the `alga_*` tools are in the tool profile's also-allow list:

```json
{
  "tools": {
    "profile": "coding",
    "alsoAllow": [
      "alga_resolve_alert",
      "alga_reopen_alert",
      "alga_promote_to_incident",
      "alga_set_outcome",
      "alga_cancel_investigation",
      "alga_pause_investigation",
      "alga_search_knowledge",
      "alga_get_knowledge",
      "alga_create_knowledge",
      "alga_list_alerts",
      "alga_triage_feedback",
      "alga_get_incident_context",
      "alga_get_incident_timeline",
      "alga_add_incident_timeline",
      "alga_who_is_on_call",
      "alga_list_services",
      "alga_search_memories",
      "alga_create_memory",
      "alga_peer_ask",
      "alga_assign_investigation",
      "alga_set_incident_priority",
      "alga_set_incident_severity",
      "alga_trigger_escalation",
      "alga_mitigate_incident",
      "alga_resolve_incident",
      "alga_begin_triage",
      "alga_promote_incident",
      "alga_assign_incident_role",
      "alga_post_handoff",
      "alga_publish_status_update",
      "alga_set_incident_resolution_docs"
    ]
  }
}
```

The plugin calls `api.runtime.config.mutateConfigFile()` in `registerFull` to merge these automatically. Restart the OpenClaw gateway after configuration.

### 5. Verify

Trigger a test alert in Alga. The agent should receive the investigation, post findings to the thread, and be able to call tools like `alga_resolve_alert`.

## Agent Tools

The plugin registers 36 agent tools (generated from `src/agent-tools.ts` — update the registry first, then this table):

| Category                    | Tools                                                                                                                                                                                                                                                                                                       |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Alert actions**           | `alga_resolve_alert`, `alga_reopen_alert`                                                                                                                                                                                                                                                                   |
| **Investigation lifecycle** | `alga_cancel_investigation`, `alga_pause_investigation`, `alga_set_outcome`, `alga_assign_investigation`                                                                                                                                                                                                    |
| **Query**                   | `alga_list_alerts`                                                                                                                                                                                                                                                                                          |
| **Knowledge**               | `alga_search_knowledge`, `alga_get_knowledge`, `alga_create_knowledge`                                                                                                                                                                                                                                      |
| **Triage**                  | `alga_triage_feedback`                                                                                                                                                                                                                                                                                      |
| **Incident (commander)**    | `alga_set_incident_priority`, `alga_set_incident_severity`, `alga_trigger_escalation`, `alga_mitigate_incident`, `alga_resolve_incident`, `alga_begin_triage`, `alga_promote_incident`, `alga_assign_incident_role`, `alga_post_handoff`, `alga_publish_status_update`, `alga_set_incident_resolution_docs` |
| **Incident (general)**      | `alga_get_incident_context`, `alga_get_incident_timeline`, `alga_add_incident_timeline`, `alga_promote_to_incident`                                                                                                                                                                                         |
| **On-call**                 | `alga_who_is_on_call`, `alga_list_services`                                                                                                                                                                                                                                                                 |
| **Memory**                  | `alga_search_memories`, `alga_create_memory`                                                                                                                                                                                                                                                                |
| **Peer**                    | `alga_peer_ask`                                                                                                                                                                                                                                                                                             |

> The `alga_request_status_update` tool and the coordination-task subsystem (`alga_dispatch_task`, `alga_claim_task`, `alga_complete_task`, `alga_list_tasks`, `alga_synthesize_findings`) have been removed — delegate via @mentions in the coordination thread and hand work back with `alga_post_handoff`.
>
> **Investigation addressing.** Alga addresses alert investigation threads as `alert_<number>` (e.g. `alert_42`) and incident threads as `incident_coord_<number>` / `incident_inv_<number>`. Pass these chat ids as the `investigation_id` argument. A bare number is treated as an alert number. There is no separate investigations REST endpoint; investigation state is driven through the `inv_tool` commands on `POST /api/v1/agent/messages`.

## Environment Variables

| Variable             | Description                                                  |
| -------------------- | ------------------------------------------------------------ |
| `ALGA_SERVER_URL`    | Base URL of the Alga backend (no trailing slash)             |
| `ALGA_AGENT_TOKEN`   | Agent bot token from the Alga UI                             |
| `ALGA_ALLOWED_USERS` | Comma-separated OpenClaw user IDs allowed to use the channel |

## Architecture

The plugin uses a channel plugin architecture (`defineChannelPluginEntry` + `createChatChannelPlugin`):

- **Inbound:** SSE connection to `GET /api/v1/agent/events` — receives investigation dispatches (`message`), operator messages, and lifecycle signals (`investigation_status_changed` for pause/cancel, `investigation_resume`), plus peer-ask/finding frames.
- **Outbound:** REST calls to `POST /api/v1/agent/messages` — sends text messages (`kind: "text"`) and investigation commands (`kind: "inv_tool"`). `text` and `inv_tool` are mutually exclusive per message.
- **Chat addressing:** alert investigations are `alert_<number>`, incident threads are `incident_coord_<number>` / `incident_inv_<number>`, and the operator DM is a fixed id.
- **Heartbeat:** `POST /api/v1/agent/heartbeat` every 30s to renew SSE presence (the backend also sends a 30s keepalive comment).
- **Streaming:** Draft preview messages during agent thinking, finalized on completion.
- **Auth:** `Authorization: Bearer alga_agent_…` on every call. Agent tokens must carry the right capabilities (`investigate`, `command`, `communicate`) for the operations they perform.

## Development

```bash
cd integrations/alga-openclaw-plugin
npm install
npm run build        # tsdown bundles to dist/
```

Install into OpenClaw for testing:

```bash
openclaw plugins install -l ./integrations/alga-openclaw-plugin
```
