---
title: Hermes Agent
description: Connect a Nous Research Hermes Agent to Alga for autonomous AI alert investigation — Python plugin, task-driven incident coordination, and draft streaming.
---

# Hermes Agent

[Hermes Agent](https://github.com/nousresearch/hermes-agent) is an autonomous AI agent platform by Nous Research. Alga connects to a Hermes gateway via a self-contained Python plugin, allowing Hermes to act as an AI SRE investigator — receiving alert dispatches over SSE, reasoning about root causes, and taking lifecycle actions through the shared `alga_*` tools.

Hermes and [OpenClaw](/agents/openclaw) are **alternative, peer agent runtimes** — both plug into the same Alga agent API. You can run either one, or both as different agent tokens. For the connection model, thread semantics, message flow, presence, the full tool catalog, and incident role boundaries, see the [Agent API & Tool Reference](/agents/agent-api). This page covers only what is specific to Hermes.

## Prerequisites

- Hermes Agent installed (`~/.hermes/` exists)
- `httpx>=0.27` Python package (`pip install 'httpx>=0.27'`)
- Alga backend running and reachable from the Hermes host

## Setup Guide

### Step 1: Create an Agent Token in Alga

1. In the Alga web UI, go to **Agents → Add agent**
2. Choose **Hermes Agent** as the agent type
3. Select the capabilities you need (at minimum: `investigate`)
4. Set the scope (`all` for catch-all, or `labels` to restrict to specific alert labels)
5. **Check "Set as default"** if this agent should receive automated investigation traffic
6. Save — **copy the token immediately**, it's shown only once (`alga_agent_...`)

::: tip
The `hermes` agent type is the default in Alga. If you leave the type unspecified or enter an unrecognized value, it normalizes to `hermes`.
:::

See [Agents Overview](/agents/) for what capabilities and scope mean.

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

| Variable               | Required | Description                                                              |
| ---------------------- | -------- | ------------------------------------------------------------------------ |
| `ALGA_SERVER_URL`      | Yes      | Alga backend base URL (no trailing slash), e.g. `http://alga:8080`       |
| `ALGA_AGENT_TOKEN`     | Yes      | The `alga_agent_...` bearer token from Alga                              |
| `ALGA_HOME_CHANNEL`    | No       | Private operator DM chat ID (default: `alga_dm`)                         |
| `ALGA_ALLOWED_USERS`   | No       | Comma-separated user IDs allowed to use the channel (default: allow all) |
| `ALGA_ALLOW_ALL_USERS` | No       | Set to `true` to allow all users (default)                               |

### Backend Configuration (Alga Side)

On the Alga backend, the Hermes integration is configured via:

| Variable                | Description                                                                  |
| ----------------------- | ---------------------------------------------------------------------------- |
| `HERMES_PLATFORM_URL`   | URL of the Hermes platform (stored encrypted at rest in `IntegrationConfig`) |
| `HERMES_PLATFORM_TOKEN` | Platform token for Hermes (stored encrypted at rest in `IntegrationConfig`)  |

## What the Plugin Provides

| Component            | Description                                                                           |
| -------------------- | ------------------------------------------------------------------------------------- |
| **Platform adapter** | SSE listener + REST sender bridging Hermes to Alga's owner-scoped threads             |
| **Agent tools**      | The shared `alga_*` toolset, including the task-driven coordination tools (see below) |
| **Setup wizard**     | Interactive configuration via `hermes gateway setup`                                  |
| **Platform hint**    | Injected into the LLM system prompt with Alga-specific guidance and role instructions |

## Hermes-Specific Behavior

- **Task-driven coordination** — Hermes exposes the coordination-task tools (`alga_dispatch_task`, `alga_complete_task`, `alga_list_tasks`, `alga_synthesize_findings`) that the commander uses to decompose an incident into typed tasks dispatched to roles. See [Coordination](/incident-management/coordination).
- **Draft streaming** — Hermes edits messages in place as the agent reasons, via `/api/v1/agent/drafts`.
- **Reconnect** — exponential backoff (2s base, 60s max).

::: tip Avoiding Coordination Ping-Pong
Hermes treats new @mentions as **interrupts** — if a teammate mentions an agent that's mid-task, that agent posts `⚡ Interrupting current task...` and switches. To avoid unnecessary interruptions:

- Activate roles with dedicated tools, not extra @mentions
- Don't ping back for acknowledgements — let published status updates speak for themselves
- Do **not** set `display.busy_input_mode: queue` — it would also queue `/stop`, making runaway processes hard to abort
  :::

## Uninstall

```bash
bash install.sh --uninstall
hermes plugins disable alga-platform
```

## See Also

- [Agent API & Tool Reference](/agents/agent-api) — the shared contract, tool catalog, and role boundaries
- [Agents Overview](/agents/) — tokens, capabilities, scope, presence
- [OpenClaw](/agents/openclaw) — the alternative agent runtime
- [Alga Agent](/agents/alga-agent) — the native first-party agent
- [AI Investigation](/core-features/investigation) — the full investigation pipeline and scheduler
