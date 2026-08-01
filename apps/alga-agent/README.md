# Alga Agent

A Go-based AI SRE assistant for the [Alga](../..) AIOps platform. It triages
alerts, investigates incidents, and runs operations through the **Alga**
channel (investigation threads), powered by an OpenAI-compatible LLM with a
tool-calling loop.

## Features

- **Alga channel**: SSE investigation threads
- **MCP integration**: expose every agent tool as an MCP server for Claude
  Desktop, Cursor, and other MCP-compatible clients. Consume external MCP
  servers (filesystem, GitHub, database, in-house) as agent tools.
- **29 Alga tools**: alerts, investigations, incidents, knowledge, memory,
  services, on-call, coordination tasks (dispatch/claim/complete/synthesize)
- **Typed tool framework**: JSON Schemas are auto-generated from Go struct
  tags — no more hand-written `map[string]any` schemas. Includes a standard
  `{ok, data, error}` result envelope and panic recovery.
- **Shell tool**: whitelisted command execution (not a sandbox — restrict the list)
- **Web search**: DuckDuckGo (default), Brave, or Tavily
- **Streaming**: typing indicators on Alga
- **Session memory**: per-chat ring buffer with idle eviction
- **Idempotency-Key injection**: every state-changing SDK call is replay-safe
  — a transient 503 retry no longer double-fires a mutation
- **Zero-external-dep metrics**: Prometheus text format on `/metrics`
- **Graceful shutdown**: SIGINT/SIGTERM with 10s drain

## Quick Start

### Install

```bash
# Latest release binary → ~/.local/bin (linux/darwin, amd64/arm64).
# Adds ~/.local/bin to PATH for bash/zsh.
curl -fsSL https://raw.githubusercontent.com/hahnavi/alga/main/scripts/install-agent.sh | bash
```

### Option A — Interactive setup (recommended)

```bash
# Full menu — configure model, channels, tools, behavior, and logging.
alga-agent setup

# Or jump straight to one section.
alga-agent setup model
alga-agent setup channel
alga-agent setup tools
alga-agent setup behavior
alga-agent setup logging
```

The wizard covers every section of `config.yaml`. The main menu shows a live
status badge for each area (e.g. `alga on`), so you can see at a
glance what's configured. Before saving, a **Review & Save** step prints a full
summary — secrets are shown only as `✓ set` / `✗ not set`, never their values —
and runs `config.Validate()` so a broken config is caught before it's written,
not at agent startup.

The wizard is arrow-key driven: navigate menus with `↑`/`↓` (or `j`/`k`),
toggle yes/no with `←`/`→` (or `y`/`n`), confirm with `Enter`, and cancel with
`Esc`/`Ctrl+C`. It writes `~/.alga/config.yaml` (mode 0600) with your values —
including secrets, which are stored literally like hermes-agent's config.
Current values are shown as defaults for free-text fields; press Enter to keep
them. Existing configs are backed up to `config.yaml.bak.<timestamp>` before any
change. Override the data dir with `ALGA_AGENT_HOME`. When stdin is not a TTY
(CI, piped input, screen readers) the wizard falls back to numbered and y/n text
prompts.

```bash
# 2. Run.
alga-agent
```

### Option B — Manual config

```bash
# 1. Copy and edit config (or use env vars exclusively).
cp config.yaml.example config.yaml

# 2. Set required secrets via env vars.
export OPENROUTER_API_KEY="sk-or-..."    # or OPENAI_API_KEY
export ALGA_SERVER_URL="http://localhost:8080"
export ALGA_AGENT_TOKEN="alga_..."

# 3. Enable the channel in config.yaml or via env:
export ALGA_ALGA_ENABLED=true

# 4. Run.
go run .
```

## Configuration

Configuration is loaded from `config.yaml`, resolved in this order: an explicit
path, `$ALGA_AGENT_CONFIG`, `./config.yaml` (back-compat), `$ALGA_AGENT_HOME/
config.yaml`, or `$HOME/.alga/config.yaml`. `${VAR}` environment variable
expansion is supported. **Environment variables always override YAML values**
— keep secrets in env vars, structure in YAML.

| Variable                    | Required        | Description                                                                                                                                                                                         |
| --------------------------- | --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `OPENROUTER_API_KEY`        | Yes*            | LLM API key (default OpenRouter provider)                                                                                                                                                           |
| `OPENAI_API_KEY`            | Yes*            | LLM API key alias (OPENROUTER_API_KEY wins when both set)                                                                                                                                           |
| Provider keys               | No              | Per-provider keys used when `model.provider` matches: `OPENCODE_ZEN_API_KEY`, `OPENCODE_GO_API_KEY`, `ZAI_API_KEY`/`GLM_API_KEY`/`Z_AI_API_KEY`, `DASHSCOPE_API_KEY`, `ALIBABA_CODING_PLAN_API_KEY` |
| `ALGA_SERVER_URL`           | If Alga enabled | Alga server URL                                                                                                                                                                                     |
| `ALGA_AGENT_TOKEN`          | If Alga enabled | Alga agent authentication token                                                                                                                                                                     |
| `SEARCH_API_KEY`            | If Brave/Tavily | Web search API key                                                                                                                                                                                  |
| `ALGA_AGENT_CONFIG`         | No              | Path to config.yaml                                                                                                                                                                                 |
| `ALGA_AGENT_HOME`           | No              | Data dir (default `~/.alga`); config lives at `<dir>/config.yaml`                                                                                                                                   |
| `ALGA_AGENT_NONINTERACTIVE` | No              | Set to `1` to make `setup` refuse to run (non-TTY guard)                                                                                                                                            |
| `ALGA_ALGA_ENABLED`         | No              | Enable Alga channel (`true`/`false`)                                                                                                                                                                |

See [`config.yaml.example`](./config.yaml.example) for the full schema.

## MCP Integration

The agent speaks the [Model Context Protocol](https://modelcontextprotocol.io)
both ways.

### Expose tools to MCP clients

```yaml
mcp:
  server:
    enabled: true
    addr: "127.0.0.1:8085"
    path: "/mcp"
```

Now Claude Desktop, Cursor, or any MCP-compatible client can connect to
`http://localhost:8085/mcp` and call `alga_list_alerts`, `alga_resolve_alert`,
`alga_dispatch_task`, `shell`, `web_search`, and every other agent tool.

### Consume external MCP servers

```yaml
mcp:
  clients:
    # Stdio transport (local subprocess MCP servers)
    - name: filesystem
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
      init_timeout: 15s

    - name: github
      command: npx
      args: ["-y", "@modelcontextprotocol/server-github"]
      env: ["GITHUB_PERSONAL_ACCESS_TOKEN=ghp_..."]

    # HTTP transport (remote MCP servers)
    - name: db
      url: https://db-mcp.internal.example.com/mcp
```

Each imported server's tools are surfaced under a namespaced name
(`<server>_<tool>`, e.g. `filesystem_read_file`, `github_list_repos`). The
LLM sees them alongside the Alga tools and calls them transparently.

## Architecture

```
┌──────────┐  ┌───────────────────┐
│   Alga   │  │   Agent Core      │
│ Channel  │  │  ┌─────────────┐  │
│ (SSE +   │  │  │ LLM Client  │  │
│  REST)   │  │  │ (OpenRouter)│  │
└────┬─────┘  │  └──────┬──────┘  │
     │        │  ┌──────▼──────┐  │
     ▼        │  │ Tool Router │  │
┌────────────────┐ │ (29 tools)  │  │
│ Message Router │◄┤  └─────────────┘  │
│ (sessions)     │ └─────────┬─────────┘
└────────────────┘           │
                             ▼
                              ┌─────────────────┐
       External MCP servers   │   MCP Layer     │   External MCP clients
       (filesystem, GitHub,   │  Server  Client │   (Claude Desktop, Cursor)
        database, in-house)   │  ▲          ▲   │
                 └────────────│  │          │   │────────┐
                              └──┴──────────┴───┘        │
                                       ▲                 │
                              ┌────────┴────────┐        │
                              │  Alga Registry  │────────┘
                              │  (29 tools)     │
                              └─────────────────┘
```

The agent is both a producer and consumer of MCP tools:

- **Server** (top-right): exposes every agent tool — `alga_*`, `shell`,
  `web_search` — to external MCP clients over Streamable HTTP at
  `http://<addr>/mcp`. Claude Desktop, Cursor, or any MCP-compatible client
  can list and call tools.
- **Client** (top-left): connects to external MCP servers — official
  filesystem / SQLite / GitHub servers, or any in-house MCP server — and
  imports their tools as agent tools. Each imported tool is namespaced
  (`<server>_<tool>`) to avoid collisions.

### Conversation Loop

```
receive → load session → build prompt → call LLM (non-streaming)
  → if tool_calls: execute → append results → repeat
  → else: stream final response → save session → send to channel
```

Tool-call turns use non-streaming requests to guarantee complete `tool_calls`
payloads (delta-merged streaming is unreliable across providers). Only the
final no-tool turn is streamed for progressive delivery (SPEC §5.2.1).

### ID Resolution

The agent never infers IDs. In Alga threads, IDs come from the investigation
context. Users provide IDs in canonical prefixed form:
`inv_<id>` (investigations), `inc_<id>` (incidents), `alert:<fingerprint>`.

## Development

```bash
# Build
go build -ldflags '-s -w' -o alga-agent .

# Test (unit tests with mock LLM/Alga servers)
go test ./...

# Vet and format
go vet ./...
gofmt -w .
```

### Full-Stack E2E Test

An opt-in, local-only test in `e2e/` runs the real agent (real LLM, AlgaChannel
over SSE) against a locally running Alga backend and asserts it replies in an
alert investigation thread. It is skipped unless `ALGA_AGENT_E2E=1`, so plain
`go test ./...` stays fast and offline. It is not intended for CI.

```bash
# 1. Start the ephemeral backend stack (from the repo root)
docker compose -f docker-compose.e2e.yml up --build -d --wait

# 2. Run the test (from apps/alga-agent)
ALGA_AGENT_E2E=1 OPENROUTER_API_KEY=sk-... OPENAI_MODEL=<model> \
  go test ./e2e/... -v -timeout 15m -count=1
# or: OPENROUTER_API_KEY=... OPENAI_MODEL=<model> moon run alga-agent:test-e2e

# 3. Tear down / reset
docker compose -f docker-compose.e2e.yml down -v
```

| Env var                                 | Required | Meaning                                                                              |
| --------------------------------------- | -------- | ------------------------------------------------------------------------------------ |
| `ALGA_AGENT_E2E=1`                      | yes      | opt-in gate; otherwise the test skips                                                |
| `OPENAI_API_KEY` / `OPENROUTER_API_KEY` | yes      | LLM credentials (standard config env overrides)                                      |
| `OPENAI_BASE_URL`, `OPENAI_MODEL`       | no       | LLM endpoint/model; pick a capable model — weak free models flake                    |
| `ALGA_E2E_SERVER_URL`                   | no       | backend base URL (default `http://localhost:3100`, the nginx proxy of the e2e stack) |
| `ALGA_AGENT_E2E_TOOLS=1`                | no       | also assert tool effects (outcome, resolve, reopen, promote); most model-dependent   |
| `ALGA_AGENT_E2E_COORDINATION=1`         | no       | run the multi-agent coordination test (two LLM agents, full incident lifecycle)      |

Expect ~2–5 minutes per run (~5–10 with `ALGA_AGENT_E2E_TOOLS=1`). Notes:

- Assertions are behavior-loose (agent posted a thread reply, canary echo,
  backend state transitions) because real LLM output is nondeterministic.
- Tool scenarios assert real backend effects: `outcome_tool` (canary root
  cause/resolution on the investigation), `resolve_tool` + `reopen_tool`
  (alert status `resolved` → `firing` on a dedicated alert), and
  `promote_tool` (incident created with a canary title and linked to the
  alert).
- If another agent is connected to the same stack, the scheduler may dispatch
  the investigation to it instead — use the ephemeral e2e stack exclusively.
- The backend port is not published by `docker-compose.e2e.yml`; the test goes
  through nginx on `:3100`, which proxies SSE fine (15s keepalives). If SSE
  ever misbehaves, publish `ports: ["18080:8080"]` on the backend service and
  set `ALGA_E2E_SERVER_URL=http://localhost:18080`.

### Multi-Agent Coordination E2E Test

A separate opt-in test (`TestMultiAgentCoordination`) runs two agents — a
commander (`command` capability) and a responder (`investigate` capability) —
and asserts the full incident coordination flow: ICS role auto-assignment,
coordination task dispatch/completion, and incident lifecycle transitions
(detected → active → mitigated → resolved).

```bash
ALGA_AGENT_E2E=1 ALGA_AGENT_E2E_COORDINATION=1 \
  OPENROUTER_API_KEY=sk-... OPENAI_MODEL=<model> \
  go test ./e2e/... -run TestMultiAgentCoordination -v -timeout 20m -count=1
```

Expect ~5–10 minutes. This test is highly model-dependent: both agents must
correctly use their coordination tools (dispatch_task, claim_task,
complete_task, mitigate_incident, set_incident_resolution_docs,
resolve_incident) in response to prompts.

### Project Structure

```
apps/alga-agent/
├── main.go                      # Entry point, signal handling, wiring
├── internal/
│   ├── config/                  # YAML + env config loading, Save/Load, data-dir resolution
│   ├── setup/                   # `alga-agent setup` interactive wizard (model + channels)
│   ├── logging/                 # slog JSON logger + redaction
│   ├── llm/                     # OpenAI-compatible client (streaming, retries, Provider interface)
│   ├── tools/
│   │   ├── registry.go          # Tool interface + capability-filtered registry
│   │   ├── typed.go             # TypedTool generic framework + Result envelope
│   │   ├── schema.go            # JSON Schema generator from struct tags
│   │   ├── alga*.go             # 29 Alga tools (alerts, investigations, incidents, tasks)
│   │   ├── shell.go             # Shell execution (allowlisted)
│   │   └── websearch.go         # DuckDuckGo/Brave/Tavily
│   ├── mcp/
│   │   ├── server.go            # Expose agent tools as MCP (Streamable HTTP)
│   │   └── client.go            # Consume external MCP servers as agent tools
│   ├── agent/
│   │   ├── agent.go             # Conversation loop
│   │   ├── prompt.go            # System prompt assembly
│   │   └── memory.go            # Session store (ring buffer)
│   ├── channels/
│   │   ├── channel.go           # Channel interface
│   │   ├── router.go            # Message dispatch
│   │   └── alga.go              # Alga adapter (context-cancel-aware)
│   └── metrics/                 # Prometheus text format
├── Dockerfile
├── config.yaml.example
└── moon.yml
```

## Docker

Build from the repository root (the build context needs the local SDK):

```bash
docker build -t alga-agent -f apps/alga-agent/Dockerfile .
docker run --rm \
  -e OPENROUTER_API_KEY="sk-or-..." \
  -e ALGA_ALGA_ENABLED=true \
  -e ALGA_SERVER_URL="http://localhost:8080" \
  -e ALGA_AGENT_TOKEN="..." \
  alga-agent
```

## Run as a systemd user service (Linux)

Install the agent as a systemd **user** service so it starts on login and
restarts on failure:

```bash
# Build to a stable location first (a /tmp binary is refused).
go build -o ~/.local/bin/alga-agent .

# Write ~/.config/systemd/user/alga-agent.service, enable, and start it.
alga-agent service install

# Manage it.
alga-agent service status
alga-agent service restart
alga-agent service stop
alga-agent service uninstall
```

`install` also enables lingering (`loginctl enable-linger`) so the service
keeps running after you log out; if that fails it prints the manual command.
Flags: `--force` overwrites a differing unit file, `--enable=false` skips
start-on-login, `--now=false` skips the immediate start.

Logs go to the journal: `journalctl --user -u alga-agent -f`.

## Data Storage

The agent keeps its state under `~/.alga` (override with `ALGA_AGENT_HOME`):

- **Sessions** — `~/.alga/sessions/*.json`, one file per conversation, written
  after every turn (mode 0600). Conversations survive restarts and idle
  eviction; they reload lazily on the next message. `/clear` deletes the file.
  Configure via `sessions:` — `persist: false` disables, `dir` overrides the
  location, `retention_days: N` prunes files older than N days (0 = keep
  forever).
- **Logs** — `~/.alga/logs/agent.log`, size-rotated (default 5 MB × 3 backups
  via `logging.max_size_mb` / `logging.backup_count`) and tee'd to stderr so
  journald capture keeps working. Set `logging.file: "stderr"` to disable file
  logging, or point `logging.file` at a custom path.

## Security Notes

- The **shell tool is not a sandbox**. Commands run with the agent's process
  privileges. Restrict `allowed_commands` and run the binary under a
  least-privilege user/container.
- Secrets are never logged. The LLM client redacts `Authorization` headers.
- The agent never persists plaintext secrets — API keys live in env vars only.

## Error Policy

- **LLM errors**: retried up to 3× with exponential backoff (1s, 2s, 4s). On
  unrecoverable failure, a generic "I'm having trouble thinking right now"
  message is sent.
- **Tool errors**: returned to the LLM as tool-result text; the LLM decides
  whether to retry or report.
- **Channel errors**: after 5 consecutive failures, the channel is marked
  unhealthy and disabled for the session.

## License

Same as the parent repository.
