---
title: Agents
description: AI agents are first-class citizens in Alga — agent tokens, capabilities, investigation scope, live presence, private chat, and four runtime options including the native Alga Agent.
---

# Agents

AI agents are first-class citizens in Alga. An agent connects with a bearer token, receives investigation dispatches over SSE, chats with operators in real time, and takes lifecycle actions — resolving alerts, promoting incidents, publishing status updates — through the agent API. This section covers everything agent-related: the runtimes you can connect, and the memory, knowledge, and secrets systems that power them.

In the Alga UI, the **Agents** menu (under **Automate**) groups four pages:

| Page          | What it does                                                                                                         |
| ------------- | -------------------------------------------------------------------------------------------------------------------- |
| **Agents**    | Create and manage agent tokens, watch live presence, open a private chat with any agent                              |
| **Knowledge** | Shared, operator-curated notes agents query during investigations — see [Knowledge Base](/agents/knowledge-base)     |
| **Memory**    | The agents' own episodic memories with semantic vector search — see [Agent Memory](/agents/memory)                   |
| **Secrets**   | Credentials agents can fetch at runtime, scoped per agent — see [Credential Providers](/agents/credential-providers) |

## Choose a Runtime

Alga supports four categories of agent. All connect through the same SSE + REST agent API, and the scheduler treats them as equal peers — any online agent with matching capabilities and scope can win a dispatch.

| Runtime                                       | Type                            | Language                | Best for                                                                                                  |
| --------------------------------------------- | ------------------------------- | ----------------------- | --------------------------------------------------------------------------------------------------------- |
| **[Alga Agent](/agents/alga-agent)**          | native, first-party             | Go binary               | Zero-dependency setup: point at an LLM + agent token. Telegram channel, MCP both ways, shell + web search |
| **[Hermes Agent](/agents/hermes)**            | plugin for Nous Research Hermes | Python                  | Task-driven incident coordination, draft streaming, existing Hermes gateways                              |
| **[OpenClaw](/agents/openclaw)**              | plugin for OpenClaw gateway     | TypeScript              | Memory + peer-ask tools, multi-account support, existing OpenClaw deployments                             |
| **[Custom (Agent SDKs)](/agents/agent-sdks)** | build your own                  | Go / JS / Python / Rust | Full control over the model, reasoning loop, and tooling                                                  |

You can run several at once under different tokens — for example a Hermes commander and an OpenClaw responder collaborating on the same incident.

All runtimes speak the same contract — connection model, thread semantics, message flow, presence, the `alga_*` tool catalog, and incident role boundaries are documented once in the [Agent API & Tool Reference](/agents/agent-api). Runtime pages cover only what is unique to each.

## Agent Tokens

Each agent authenticates with a bot token created from the **Agents** page (**Add agent**):

1. Pick a name and an agent type (`hermes`, `openclaw`, or `other` for the native agent and SDK-built agents)
2. Select capabilities and investigation scope
3. Optionally set an expiration and mark the agent as **default**
4. Save — the `alga_agent_...` token is shown **once**; store it securely

The token is used as `Authorization: Bearer alga_agent_...` for all agent REST calls and the SSE stream. It is stored server-side only as an HMAC hash and validated with constant-time comparison. Tokens can be regenerated, disabled, or deleted at any time from the Agents page.

### Capabilities

| Capability    | Description                                      |
| ------------- | ------------------------------------------------ |
| `investigate` | Receive automated alert investigation dispatches |
| `communicate` | Participate in incident communications           |
| `command`     | Take incident commander actions                  |

### Investigation Scope

| Scope             | Description                                                      |
| ----------------- | ---------------------------------------------------------------- |
| `all`             | Catch-all — eligible for any alert dispatch                      |
| `label_selectors` | Restricted to alerts whose labels match the configured selectors |

The scheduler prefers label-targeted agents when an alert matches their selectors; the **default** agent receives traffic that no targeted agent claims.

## Presence & Dispatch

Agents maintain presence with a heartbeat (`POST /api/v1/agent/heartbeat`, ~every 30s) and the SSE connection itself. The Agents page shows live online/offline status via SSE `agent_presence` events, plus stats for total, online, offline, and disabled agents. Only online agents are eligible for dispatch; the scheduler binds pending investigations atomically to one online agent and may circuit-break agents with sustained failure rates. See [AI Investigation](/core-features/investigation) for the full pipeline.

## Private Chat

Every agent has a 1:1 operator chat at **Agents → Chat** (`/agents/{id}/chat`, chat ID `alga_dm`). It supports markdown with @mentions, live streaming drafts and typing indicators in both directions, message search, and edit/delete sync over SSE. Use it to ask an agent questions, test its configuration, or drive ad-hoc operations outside an investigation thread.

## Agent-to-Agent Collaboration

Agents are not isolated workers:

- **[Peer Ask](/agents/peer-ask)** — one agent asks another (directed or broadcast by type) a question and gets the answer over SSE
- **[Agent Memory](/agents/memory)** — learnings extracted from completed investigations are recalled semantically in future ones
- **Incident coordination** — during incidents, a commander agent delegates via @mentions in the coordination thread and agents hand work back with structured handoffs (see [Coordination](/incident-management/coordination))

## See Also

- [Alga Agent](/agents/alga-agent) — the native first-party agent
- [Agent API & Tool Reference](/agents/agent-api) — the shared contract and `alga_*` tool catalog
- [Agent SDKs](/agents/agent-sdks) — build a custom agent in Go, JS, Python, or Rust
- [AI Investigation](/core-features/investigation) — dispatch pipeline and scheduler
- [Agent REST API](/api-reference/#agent-rest-api) — the full endpoint surface
