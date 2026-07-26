---
title: Alga Agent
description: Alga's first-party Go-based AI SRE agent — dual-channel Telegram + Alga investigation threads, 29 Alga tools, shell and web search, and two-way MCP integration.
---

# Alga Agent

Alga Agent (`apps/alga-agent`) is the **native, first-party AI SRE agent** that ships with Alga. It is a standalone Go binary that triages alerts, investigates incidents, and runs operations through two independent channels — **Telegram** (human interface) and **Alga** (investigation threads over SSE + REST) — powered by any OpenAI-compatible LLM with a tool-calling loop.

Unlike the [Hermes](/agents/hermes) and [OpenClaw](/agents/openclaw) plugins, which bridge Alga to external agent platforms, Alga Agent needs no third-party gateway: point it at an LLM endpoint and an agent token and it's ready.

## Features

- **Dual-channel** — Telegram (long polling or webhook) + Alga SSE investigation threads
- **8 LLM providers** — OpenRouter (default), OpenAI, OpenCode Zen, OpenCode Go, Z.AI, Z.AI Coding Plan, Alibaba (DashScope), Alibaba Coding Plan, plus any custom OpenAI-compatible endpoint
- **29 Alga tools** — alerts, investigations, incidents, knowledge, memory, services, on-call, and coordination tasks (dispatch/claim/complete/synthesize)
- **MCP both ways** — expose every agent tool as an MCP server for Claude Desktop, Cursor, and other MCP clients; consume external MCP servers (filesystem, GitHub, database, in-house) as agent tools
- **Shell tool** — allowlisted command execution (not a sandbox — restrict the list)
- **Web search** — DuckDuckGo (default), Brave, or Tavily
- **Streaming** — progressive message edits on Telegram; typing indicators on Alga
- **Session persistence** — per-chat ring buffer with idle eviction, persisted to disk with rotating logs
- **Replay-safe mutations** — an `Idempotency-Key` is injected on every state-changing SDK call, so a transient 503 retry never double-fires
- **systemd service** — install and manage as a user service (`alga-agent service install`)
- **Prometheus metrics** on `/metrics`, graceful shutdown with a 10s drain

## Quick Start

### Install

```bash
# Latest release binary → ~/.local/bin (linux/darwin, amd64/arm64).
# Verifies the SHA256 checksum and adds ~/.local/bin to PATH for bash/zsh.
curl -fsSL https://raw.githubusercontent.com/hahnavi/alga/main/scripts/install-agent.sh | bash
```

Pre-built binaries and Docker images are published on every `agent-v*` tag; see [Releases](https://github.com/hahnavi/alga/releases?q=agent-v).

### Step 1: Create an Agent Token in Alga

1. In the Alga web UI, go to **Agents → Add agent**
2. Choose **Other (Agent SDK / Self-developed)** as the agent type
3. Select the capabilities you need (at minimum: `investigate`)
4. Set the scope and default flag as needed
5. Save — **copy the token immediately**, it's shown only once (`alga_agent_...`)

### Step 2: Interactive Setup (Recommended)

```bash
# Full menu — configure model, channels, tools, behavior, and logging.
alga-agent setup

# Or jump straight to one section.
alga-agent setup model
alga-agent setup channel
alga-agent setup tools
```

The arrow-key-driven wizard covers every section of `config.yaml`, shows a live status badge per area (e.g. `telegram on · alga off`), prints a **Review & Save** summary (secrets shown only as `✓ set` / `✗ not set`), and validates the config before writing `~/.alga/config.yaml` (mode 0600). Existing configs are backed up before any change.

The **Model & Provider** step lists 8 known providers with their canonical endpoints — OpenRouter is the default and offers a curated model picker that merges a hand-picked list with a live fetch of the provider's `/models` endpoint (filtered to tool-capable models, with an offline curated fallback). Switching providers resets the model and suggests the canonical base URL, so switching from OpenRouter to Z.AI or Alibaba is one menu pick.

```bash
# Run.
alga-agent
```

### Manual Config (Alternative)

```bash
cp config.yaml.example config.yaml

export OPENROUTER_API_KEY="sk-or-..."    # or OPENAI_API_KEY
export TELEGRAM_BOT_TOKEN="123:abc..."   # if telegram enabled
export ALGA_SERVER_URL="http://localhost:8080"
export ALGA_AGENT_TOKEN="alga_agent_..."
export ALGA_TELEGRAM_ENABLED=true
export ALGA_ALGA_ENABLED=true

go run .
```

## Configuration

Configuration is loaded from `config.yaml` — resolved from an explicit path, `$ALGA_AGENT_CONFIG`, `./config.yaml`, `$ALGA_AGENT_HOME/config.yaml`, or `$HOME/.alga/config.yaml`. `${VAR}` expansion is supported, and **environment variables always override YAML values** — keep secrets in env vars, structure in YAML.

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENROUTER_API_KEY` | Yes* | LLM API key (default OpenRouter provider) |
| `OPENAI_API_KEY` | Yes* | LLM API key alias (`OPENROUTER_API_KEY` wins when both set) |
| Provider keys | No | Per-provider keys used when `model.provider` matches: `OPENCODE_ZEN_API_KEY`, `OPENCODE_GO_API_KEY`, `ZAI_API_KEY`/`GLM_API_KEY`/`Z_AI_API_KEY`, `DASHSCOPE_API_KEY`, `ALIBABA_CODING_PLAN_API_KEY` |
| `TELEGRAM_BOT_TOKEN` | If Telegram enabled | Telegram bot token from @BotFather |
| `ALGA_SERVER_URL` | If Alga enabled | Alga server URL |
| `ALGA_AGENT_TOKEN` | If Alga enabled | The `alga_agent_...` bearer token |
| `SEARCH_API_KEY` | If Brave/Tavily | Web search API key |
| `ALGA_AGENT_CONFIG` | No | Path to config.yaml |
| `ALGA_AGENT_HOME` | No | Data dir (default `~/.alga`) |
| `ALGA_AGENT_NONINTERACTIVE` | No | Set to `1` to make `setup` refuse to run (non-TTY guard) |
| `ALGA_TELEGRAM_ENABLED` | No | Enable Telegram channel (`true`/`false`) |
| `ALGA_ALGA_ENABLED` | No | Enable Alga channel (`true`/`false`) |

\* At least one LLM key is required. The provider field controls which canonical base URL is used when `base_url` is omitted.

See `apps/alga-agent/config.yaml.example` for the full schema.

## MCP Integration

The agent speaks the [Model Context Protocol](https://modelcontextprotocol.io) both ways.

### Expose tools to MCP clients

```yaml
mcp:
  server:
    enabled: true
    addr: "127.0.0.1:8085"
    path: "/mcp"
```

Claude Desktop, Cursor, or any MCP-compatible client can then connect to `http://localhost:8085/mcp` and call `alga_list_alerts`, `alga_resolve_alert`, `alga_dispatch_task`, `shell`, `web_search`, and every other agent tool.

### Consume external MCP servers

```yaml
mcp:
  clients:
    # Stdio transport (local subprocess MCP servers)
    - name: filesystem
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]

    # HTTP transport (remote MCP servers)
    - name: db
      url: https://db-mcp.internal.example.com/mcp
```

Each imported server's tools are surfaced under a namespaced name (`<server>_<tool>`, e.g. `filesystem_read_file`) and the LLM calls them alongside the Alga tools transparently.

## How It Connects to Alga

The Alga channel adapter connects over the same agent API used by every other runtime: SSE dispatch on `/api/v1/agent/events`, REST messages on `/api/v1/agent/messages`, and a heartbeat on `/api/v1/agent/heartbeat` to keep the presence lease alive. The agent appears on the **Agents** page with live online status, and the scheduler dispatches investigations to it like any other agent with matching capabilities and scope.

### Conversation Loop

```text
receive → load session → build prompt → call LLM
  → if tool_calls: execute → append results → repeat
  → else: stream final response → save session → send to channel
```

Tool-call turns use non-streaming requests to guarantee complete `tool_calls` payloads; only the final no-tool turn is streamed for progressive delivery.

## Docker

Build from the repository root (the build context needs the local SDK):

```bash
docker build -t alga-agent -f apps/alga-agent/Dockerfile .
docker run --rm \
  -e OPENROUTER_API_KEY="sk-or-..." \
  -e ALGA_ALGA_ENABLED=true \
  -e ALGA_SERVER_URL="http://alga:8080" \
  -e ALGA_AGENT_TOKEN="alga_agent_..." \
  alga-agent
```

Release images are published to GHCR by the `agent-release.yml` workflow on `agent-v*` tags.

## Security Notes

- The **shell tool is not a sandbox** — commands run with the agent's process privileges. Restrict `allowed_commands` and run the binary under a least-privilege user or container.
- Secrets are never logged; the LLM client redacts `Authorization` headers.
- The Telegram webhook validates its secret path segment with a constant-time compare.
- The agent never persists plaintext secrets — API keys live in env vars only.

## See Also

- [Agents Overview](/agents/) — agent tokens, capabilities, and runtime options
- [Hermes Agent](/agents/hermes) and [OpenClaw](/agents/openclaw) — external agent runtimes
- [Agent SDKs](/agents/agent-sdks) — the SDKs Alga Agent itself builds on
- [AI Investigation](/core-features/investigation) — how investigations are dispatched
