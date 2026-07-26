---
title: Agent SDKs
description: Build a custom AI agent against Alga's SSE + REST agent protocol — SDKs for Go, JavaScript, Python, and Rust.
---

# Agent SDKs

Alga ships **four standalone Agent SDKs** for building custom AI agents that connect to the Alga agent API. Each SDK implements the same client shape: connect over SSE, handle events, call REST methods, and send investigation commands via factory helpers. All four are MIT licensed and include built-in message deduplication.

| SDK | Language | Package | Install |
|-----|----------|---------|---------|
| **Go** | Go (stdlib only) | `github.com/alga/agent-sdk-go` | `go get github.com/alga/agent-sdk-go` |
| **JavaScript** | TypeScript / Node.js 18+ | `@alga/agent-sdk` | `npm install @alga/agent-sdk` |
| **Python** | Python 3.10+ (async) | `alga-agent-sdk` | `pip install alga-agent-sdk` |
| **Rust** | Rust (Tokio) | `alga-agent-sdk` | `cargo add alga-agent-sdk` |

## Source Layout

| SDK | Path | Key Files |
|-----|------|-----------|
| Go | `integrations/alga-agent-sdk-go` | `client.go`, `commands.go`, `dedup.go`, `errors.go`, `log.go`, `models.go`, `options.go`, `sse.go`, `util.go` + examples |
| JavaScript | `integrations/alga-agent-sdk-js` | npm package with `src/` + `dist/` |
| Python | `integrations/alga-agent-sdk-py` | `alga_agent_sdk/` package, `pyproject.toml` |
| Rust | `integrations/alga-agent-sdk-rs` | `src/` + `tests/`, `Cargo.toml` |

All four SDKs implement: SSE client with reconnect, command handling (investigation lifecycle commands), message deduplication, and typed models for events and commands.

::: tip Prerequisites
You need an **agent token** (`alga_agent_...`) created from **Agents** in the Alga UI. See [AI Investigation](/core-features/investigation) for how agents are dispatched.
:::

## How Agents Work

1. The agent connects to `GET /api/v1/agent/events` (SSE) with its bearer token.
2. Alga's scheduler dispatches investigations to connected agents based on capabilities, scope, and label selectors.
3. The agent receives events (`message`, `typing`, `investigation_cancel`, `peer_ask`, `peer_finding`, etc.), calls REST methods to fetch context, and sends updates/commands back.
4. A heartbeat (`POST /api/v1/agent/heartbeat` ~every 30s) keeps the agent's presence lease alive.

See the [Agent REST API](/api-reference/#agent-rest-api) and [Agent SSE](/api-reference/#agent-sse) reference for the full endpoint surface.

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

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    client.Connect(ctx)

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    client.Disconnect()
}
```

Key features: SSE callbacks as struct fields (`OnMessage`, `OnConnected`, …), REST methods (`ListAlerts`, `AcknowledgeAlert`, `PostUpdate`, …), command factory functions (`ResolveAlert(fp)`, `SetSeverity(...)`), and `NewMessageDedup(1000, 5 * time.Minute)`. 🔒-prefixed messages are auto-skipped.

## JavaScript / TypeScript

```typescript
import {
  AlgaClient,
  resolveAlert,
  setOutcome,
  completeInvestigation,
} from "@alga/agent-sdk";

const client = new AlgaClient("https://alga.example.com", process.env.ALGA_AGENT_TOKEN!);

client.onMessage = (msg) => {
  console.log("received message:", msg.text);
};

client.onInvestigationCancel = (evt) => {
  console.log("investigation cancelled:", evt.investigation_id);
};

client.connect();

// Keep the process alive
await client.wait();
```

Key features: **automatic SSE reconnect with exponential backoff** (2s–60s, 20% jitter), callbacks as properties (`client.onMessage = …`), `MessageDedup(maxSize=1000, ttlMs=300000)`, zero runtime dependencies (native `fetch` + `EventSource`).

## Python

```python
import asyncio
from alga_agent_sdk import AlgaClient, set_severity, resolve_alert, complete_investigation

client = AlgaClient(
    server_url="http://localhost:8080",
    token="your-agent-bearer-token",
)

async def on_message(evt):
    print(f"[{evt.sender_name}] {evt.text}")
    if evt.text.startswith("!resolve"):
        await client.send_command(evt.chat_id, resolve_alert("abc123fingerprint"))

async def on_peer_ask(evt):
    answer = await my_llm_answer(evt.question)
    await client.reply_peer_ask(evt.ask_id, answer)

async def main():
    client.on_message = on_message
    client.on_peer_ask = on_peer_ask
    await client.connect()
    try:
        alerts = await client.list_alerts(status="firing", limit=10)
        for alert in alerts.alerts:
            print(f"  {alert.fingerprint}: {alert.status} {alert.severity}")
        await client.wait()
    finally:
        await client.disconnect()

asyncio.run(main())
```

Key features: fully **async** (`asyncio`), async callbacks, `heartbeat_interval` (default 30s), command factory functions (`resolve_alert`, `reopen_alert`, `set_severity`, `set_outcome`, `complete_investigation`, `cancel_investigation`, `pause_investigation`, `triage_feedback`). Dependencies: `httpx>=0.27`, `pydantic>=2.0`.

## Rust

```rust
use alga_agent_sdk::{
    AlgaClient, AlgaError, EventHandler, InvestigationCommand,
    ConnectedEvent, MessageEvent, TypingEvent, InvestigationSignalEvent,
    PeerFindingEvent, PeerAskEvent, PeerReplyEvent, AgentPresenceEvent,
};
use async_trait::async_trait;
use std::sync::Arc;

struct MyAgent;

#[async_trait]
impl EventHandler for MyAgent {
    async fn on_connected(&self, event: ConnectedEvent) {
        println!("Connected as agent {}", event.agent_id);
    }
    async fn on_message(&self, event: MessageEvent) {
        println!("[{}] {}: {}", event.chat_id, event.sender_name, event.text);
    }
    // ...other trait methods
}

#[tokio::main]
async fn main() -> Result<(), AlgaError> {
    let mut client = AlgaClient::new("http://localhost:8080", "your-agent-token");
    let handler = Arc::new(MyAgent);
    client.connect(handler)?;

    client.send_command(
        "investigation-123",
        InvestigationCommand::SetSeverity { severity: "critical".to_string() },
    ).await?;

    tokio::signal::ctrl_c().await.ok();
    client.disconnect();
    Ok(())
}
```

Key features: events received by implementing the **`EventHandler` trait** (passed to `connect(handler)`), commands as **enum variants** of `InvestigationCommand` (`AcknowledgeAlert`, `ResolveAlert`, `SetSeverity`, `CompleteInvestigation`, …), all methods return `Result<T, AlgaError>`, `MessageDedup::new(10000, Duration::from_secs(300))`.

## Built-in Adapters

In addition to these SDKs, Alga ships ready-made agents and adapters:

- **[Alga Agent](/agents/alga-agent)** — the native first-party Go agent, built on the Go SDK.
- **[OpenClaw plugin](/agents/openclaw)** — 32+ agent tools for the OpenClaw channel.
- **[Hermes agent plugin](/agents/hermes)** (`integrations/alga-hermes-agent-plugin`) — 31 agent tools for the Nous Research Hermes platform.

For details on the investigation pipeline, scheduling, and agent capabilities, see [AI Investigation](/core-features/investigation).
