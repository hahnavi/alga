---
title: Agent SDKs
description: Build a custom AI agent against Alga's SSE + REST agent protocol — SDKs for Go, JavaScript, Python, and Rust.
---

# Agent SDKs

Alga ships **four standalone Agent SDKs** for building custom AI agents that connect to the Alga agent API. Each SDK implements the same client shape: connect over SSE, handle events, call REST methods, and send investigation commands via factory helpers. All four are MIT licensed and share a unified feature set:

- **SSE client** with exponential backoff reconnect (2s → 60s, jitter), `Retry-After` honoring, and terminal auth-error detection (401/403 stops the loop)
- **REST client** with configurable retries on transient errors (429, 5xx, network), `Retry-After` parsing, and `Idempotency-Key` auto-injection on `POST /messages`
- **Command builders** for all backend ops: alert lifecycle, incident lifecycle, coordination tasks, status updates, and resolution docs
- **Message deduplication** (bounded, TTL-based, no-evict-on-insert)
- **Typed models** for all SSE events and REST responses
- **`onUnknownEvent` escape hatch** for forward compatibility with new backend event types

| SDK            | Language                 | Package                        | Install                               |
| -------------- | ------------------------ | ------------------------------ | ------------------------------------- |
| **Go**         | Go (stdlib only)         | `github.com/alga/agent-sdk-go` | `go get github.com/alga/agent-sdk-go` |
| **JavaScript** | TypeScript / Node.js 18+ | `@alga/agent-sdk`              | `npm install @alga/agent-sdk`         |
| **Python**     | Python 3.10+ (async)     | `alga-agent-sdk`               | `pip install alga-agent-sdk`          |
| **Rust**       | Rust (Tokio)             | `alga-agent-sdk`               | `cargo add alga-agent-sdk`            |

## Source Layout

| SDK        | Path                             | Key Files                                                                                                                |
| ---------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Go         | `integrations/alga-agent-sdk-go` | `client.go`, `commands.go`, `dedup.go`, `errors.go`, `log.go`, `models.go`, `options.go`, `sse.go`, `util.go` + examples |
| JavaScript | `integrations/alga-agent-sdk-js` | npm package with `src/` + `dist/`                                                                                        |
| Python     | `integrations/alga-agent-sdk-py` | `alga_agent_sdk/` package, `pyproject.toml`                                                                              |
| Rust       | `integrations/alga-agent-sdk-rs` | `src/` + `tests/`, `Cargo.toml`                                                                                          |

::: tip Prerequisites
You need an **agent token** (`alga_agent_...`) created from **Agents** in the Alga UI. See [AI Investigation](/core-features/investigation) for how agents are dispatched.
:::

## How Agents Work

1. The agent connects to `GET /api/v1/agent/events` (SSE) with its bearer token.
2. Alga's scheduler dispatches investigations to connected agents based on capabilities, scope, and label selectors.
3. The agent receives events (`message`, `typing`, `coordination_task_dispatched`, `peer_ask`, `peer_finding`, etc.), calls REST methods to fetch context, and sends updates/commands back.
4. A heartbeat (`POST /api/v1/agent/heartbeat` ~every 30s) keeps the agent's presence lease alive.

See the [Agent REST API](/api-reference/#agent-rest-api) and [Agent SSE](/api-reference/#agent-sse) reference for the full endpoint surface.

## Unified SSE Events

All four SDKs handle the same event set:

| Event                          | Description                                     |
| ------------------------------ | ----------------------------------------------- |
| `connected`                    | Initial connection handshake                    |
| `message`                      | Chat message from operator or peer              |
| `typing`                       | Typing indicator                                |
| `investigation_resume`         | Investigation resumed                           |
| `peer_finding`                 | Notable finding from a peer agent               |
| `peer_ask`                     | Another agent is asking a question              |
| `peer_reply`                   | Reply to your peer ask                          |
| `coordination_task_dispatched` | Commander dispatched a coordination task to you |
| `summarize_incident`           | Backend requests an incident summary            |
| `alert_auto_resolved`          | An investigated alert auto-resolved             |
| `incident_comms_stale`         | Incident comms went quiet past SLA threshold    |
| _(any other)_                  | Routed to `onUnknownEvent` escape hatch         |

## Unified Command Builders

All four SDKs provide factory functions for every backend `inv_tool` op. Incident-scoped commands take `incident_number` (integer), matching the backend contract.

- **Alert lifecycle** — `resolve_alert`, `reopen_alert`, `set_outcome`, `cancel_investigation`, `pause_investigation`, `triage_feedback`, `assign_investigation`, `promote_to_incident`
- **Incident lifecycle** — `set_incident_priority`, `set_incident_severity`, `trigger_escalation`, `mitigate_incident`, `resolve_incident`, `begin_triage`, `promote_incident`, `assign_incident_role`
- **Coordination** — `post_handoff`, `publish_status_update`, `set_incident_resolution_docs`
- **Coordination tasks** — `dispatch_task`, `dispatch_task_to_agent`, `claim_task`, `complete_task`, `synthesize_findings`

## Go

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    alga "github.com/alga/agent-sdk-go"
)

func main() {
    serverURL := os.Getenv("ALGA_SERVER_URL")
    token := os.Getenv("ALGA_AGENT_TOKEN")

    client := alga.NewAlgaClient(serverURL, token)

    client.OnMessage = func(evt alga.MessageEvent) {
        fmt.Printf("Message: %s\n", evt.Text)
        client.SendMessage(context.Background(), evt.ChatID, "Got it!", nil)
    }

    client.OnCoordinationTask = func(evt alga.CoordinationTaskEvent) {
        fmt.Printf("Task: %s\n", evt.Goal)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    client.Connect(ctx)

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    client.Disconnect()
}
```

Key features: SSE callbacks as struct fields (`OnMessage`, `OnCoordinationTask`, `OnUnknownEvent`, …), REST methods with retry + idempotency, command factory functions (`ResolveAlert(fp)`, `DispatchTask(n, kind, goal, role)`), terminal auth errors via `Err() <-chan error`. Stdlib-only, zero dependencies.

## JavaScript / TypeScript

```typescript
import { AlgaClient, resolveAlert, dispatchTask } from "@alga/agent-sdk";

const client = new AlgaClient("https://alga.example.com", process.env.ALGA_AGENT_TOKEN!);

client.onMessage = (msg) => {
  console.log("received message:", msg.text);
};

client.onCoordinationTask = (evt) => {
  console.log("task dispatched:", evt.goal);
};

client.onErr((err) => console.error("terminal:", err.message));

client.connect();
```

Key features: automatic SSE reconnect with exponential backoff (2s–60s, jitter), `Retry-After` honoring, `Idempotency-Key` auto-injection, callbacks as properties (`client.onMessage = …`), `onUnknownEvent` escape hatch, zero runtime dependencies (native `fetch`).

## Python

```python
import asyncio
from alga_agent_sdk import AlgaClient, resolve_alert, dispatch_task

client = AlgaClient(
    server_url="http://localhost:8080",
    token="your-agent-bearer-token",
)

async def on_message(evt):
    print(f"[{evt.sender_name}] {evt.text}")

async def on_coordination_task(evt):
    print(f"Task: {evt.goal}")

async def main():
    client.on_message = on_message
    client.on_coordination_task = on_coordination_task
    await client.connect()
    try:
        alerts = await client.list_alerts({"status": "firing", "limit": "10"})
        await client.wait()
    finally:
        await client.disconnect()

asyncio.run(main())
```

Key features: fully **async** (`asyncio` + `httpx`), async callbacks, `Idempotency-Key` auto-injection, REST retries with `Retry-After`, terminal auth errors via `on_err()`. Dependencies: `httpx>=0.27`, `pydantic>=2.0`.

## Rust

```rust
use alga_agent_sdk::{AlgaClient, AlgaError, EventHandler};
use alga_agent_sdk::models::*;
use async_trait::async_trait;
use std::sync::Arc;

struct MyAgent;

#[async_trait]
impl EventHandler for MyAgent {
    async fn on_connected(&self, event: ConnectedEvent) {
        println!("Connected as agent {:?}", event.agent_id);
    }
    async fn on_message(&self, event: MessageEvent) {
        println!("[{:?}] {:?}", event.chat_id, event.text);
    }
    async fn on_coordination_task(&self, event: CoordinationTaskEvent) {
        println!("Task: {:?}", event.goal);
    }
}

#[tokio::main]
async fn main() -> Result<(), AlgaError> {
    let mut client = AlgaClient::new("http://localhost:8080", "your-agent-token")?;
    client.connect(Arc::new(MyAgent))?;

    client.send_command("alert_42", alga_agent_sdk::commands::resolve_alert("fp-abc")).await?;

    tokio::signal::ctrl_c().await.ok();
    client.disconnect();
    Ok(())
}
```

Key features: events received by implementing the **`EventHandler` trait** (all methods have default no-ops), commands as **builder functions** (`resolve_alert(fp)`, `dispatch_task(n, kind, goal, role)`), `Idempotency-Key` auto-injection, REST retries, fatal error retrieval via `take_fatal_error()`.

## Built-in Adapters

In addition to these SDKs, Alga ships ready-made agents and adapters:

- **[Alga Agent](/agents/alga-agent)** — the native first-party Go agent, built on the Go SDK.
- **[OpenClaw plugin](/agents/openclaw)** — 30+ agent tools for the OpenClaw channel, including coordination task tools.
- **[Hermes agent plugin](/agents/hermes)** (`integrations/alga-hermes-agent-plugin`) — 31 agent tools for the Nous Research Hermes platform.

For details on the investigation pipeline, scheduling, and agent capabilities, see [AI Investigation](/core-features/investigation).
