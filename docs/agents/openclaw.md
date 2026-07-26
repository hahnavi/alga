---
title: OpenClaw Agent
description: Connect an OpenClaw agent gateway to Alga for autonomous AI investigation — TypeScript plugin with built-in memory tools, peer-ask collaboration, and multi-account support.
---

# OpenClaw Agent

[OpenClaw](https://openclaw.ai) is an autonomous AI agent gateway that connects to Alga via a self-contained TypeScript channel plugin, allowing OpenClaw-powered agents to act as AI SRE investigators — receiving alert dispatches over SSE, reasoning about root causes, and taking lifecycle actions through the shared `alga_*` tools.

OpenClaw and [Hermes](/agents/hermes) are **alternative, peer agent runtimes** — both plug into the same Alga agent API. You can run either one, or both as different agent tokens. For the connection model, thread semantics, message flow, presence, the full tool catalog, and incident role boundaries, see the [Agent API & Tool Reference](/agents/agent-api). This page covers only what is specific to OpenClaw.

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

See [Agents Overview](/agents/) for what capabilities and scope mean.

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

The installer copies the plugin into `~/.openclaw/extensions/alga/`, installs runtime dependencies, and merges all `alga_*` tool names into `tools.alsoAllow` in your OpenClaw config.

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

On startup, the plugin automatically verifies that all `alga_*` tools are in your `tools.alsoAllow` list and merges any missing ones.

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

## OpenClaw-Specific Behavior

- **Memory tools** — OpenClaw exposes `alga_search_memories` and `alga_create_memory`, giving the agent its own private recall across investigations. See [Agent Memory](/agents/memory).
- **Peer-ask tool** — `alga_peer_ask` lets the agent consult another agent directly. See [Peer Ask](/agents/peer-ask).
- **Per-segment narration** — each reasoning segment is posted as its own message before the following tool call, and tool calls are narrated as `🧩 tool_name [key arg]` on their own line so operators can follow along in real time.
- **Reconnect** — fixed 5s delay; the plugin reconnects automatically when the SSE connection drops.

### Agent Behavioral Hints

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

## OpenClaw-Specific Troubleshooting

For shared issues (agent not receiving investigations, auth failures, 🔒 messages, unresolved alerts), see the [Agent API & Tool Reference](/agents/agent-api#shared-troubleshooting).

### Agent says tools aren't available

Ensure `tools.alsoAllow` in `~/.openclaw/openclaw.json` includes all `alga_*` tool names. The plugin auto-configures this on startup, but if the gateway was already running during installation, restart it:

```bash
openclaw gateway restart
```

## See Also

- [Agent API & Tool Reference](/agents/agent-api) — the shared contract, tool catalog, and role boundaries
- [Agents Overview](/agents/) — tokens, capabilities, scope, presence
- [Hermes Agent](/agents/hermes) — the alternative agent runtime
- [Agent Memory](/agents/memory) — vector-searched agent memories
- [Peer Ask](/agents/peer-ask) — agent-to-agent collaboration
- [AI Investigation](/core-features/investigation) — the full investigation pipeline and scheduler
