# alga-agent-sdk

Low-level Rust client library for building custom AI agents that integrate with the [Alga](https://github.com/anomalyco/alga) SRE investigation platform. Built on tokio + reqwest.

## Installation

```toml
[dependencies]
alga-agent-sdk = "0.3"
```

## Quickstart

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
}

#[tokio::main]
async fn main() -> Result<(), AlgaError> {
    let mut client = AlgaClient::new("https://alga.example.com", "your-agent-token")?;
    client.connect(Arc::new(MyAgent))?;

    let alerts = client.list_alerts(&[("status", "firing")]).await?;
    client.send_message("alert_42", "Investigating now.", None).await?;

    tokio::signal::ctrl_c().await.ok();
    client.disconnect();
    Ok(())
}
```

## Configuration

```rust
use alga_agent_sdk::{AlgaClient, AlgaClientOptions, MessageDedup};
use std::sync::Arc;
use std::time::Duration;

let options = AlgaClientOptions {
    heartbeat_interval: Duration::from_secs(30),
    max_rest_retries: 2,
    user_agent: "my-agent/1.0".to_string(),
    dedup: Some(Arc::new(MessageDedup::new(1000, Duration::from_secs(300)))),
};
let client = AlgaClient::with_options("https://alga.example.com", token, options)?;
```

### Options

| Option             | Type             | Default             | Description                                 |
| ------------------ | ---------------- | ------------------- | ------------------------------------------- |
| heartbeat_interval | Duration         | 30s                 | Heartbeat cadence (floor 1s)                |
| max_rest_retries   | usize            | 2                   | Max REST retry attempts on transient errors |
| user_agent         | String           | "alga-agent-sdk-rs" | User-Agent header                           |
| dedup              | Option<Arc<...>> | new(1000, 300s)     | SSE message dedup cache                     |

## SSE Events

Implement the `EventHandler` trait. All methods have default no-op implementations. The SSE client auto-reconnects with exponential backoff (2s → 60s with jitter), honoring `Retry-After` on 429s.

| Method                    | Event type             | Description                              |
| ------------------------- | ---------------------- | ---------------------------------------- |
| `on_connected`            | `connected`            | Initial connection handshake             |
| `on_message`              | `message`              | Chat message from operator/peer          |
| `on_typing`               | `typing`               | Typing indicator                         |
| `on_investigation_resume` | `investigation_resume` | Investigation resumed                    |
| `on_peer_finding`         | `peer_finding`         | Notable finding from a peer agent        |
| `on_peer_ask`             | `peer_ask`             | Another agent is asking a question       |
| `on_peer_reply`           | `peer_reply`           | Reply to your peer ask                   |
| `on_summarize_incident`   | `summarize_incident`   | Backend requests an incident summary     |
| `on_alert_auto_resolved`  | `alert_auto_resolved`  | An investigated alert auto-resolved      |
| `on_incident_comms_stale` | `incident_comms_stale` | Incident comms went quiet past threshold |
| `on_unknown_event`        | any other              | Escape hatch for new backend event types |

Terminal auth errors (401/403) stop the loops and are retrievable via `client.take_fatal_error()`.

## Commands

Commands are sent via `send_command()` using builder functions. All incident-scoped commands take `incident_number: i64`.

```rust
use alga_agent_sdk::commands::*;

client.send_command("alert_42", resolve_alert("fp-abc")).await?;
client.send_command("alert_42", set_outcome(Some("OOM"), None)).await?;
client.send_command("incident_coord_9", set_incident_priority(9, "P1")).await?;
```

## REST Methods

### Alerts

```rust
let alerts = client.list_alerts(&[("status", "firing"), ("limit", "10")]).await?;
let alert = client.get_alert("fp-abc").await?;
client.resolve_alert("fp-abc").await?;
client.reopen_alert("fp-abc").await?;
```

### Incidents

```rust
let ctx = client.get_incident(9).await?;
let timeline = client.get_incident_timeline(9).await?;
client.add_incident_timeline(9, "Root cause: disk full", "root_cause").await?;
client.update_incident_summary(9, "Summarized...").await?;
```

### Messages

```rust
let result = client.send_message("chat-1", "Investigating.", Some(&["@oncall"])).await?;
client.send_draft("chat-1", "draft-1", "partial...").await?;
client.send_typing("chat-1", true).await?;
client.edit_message("msg-9", "chat-1", "edited").await?;
client.delete_message("msg-9", "chat-1").await?;
```

### Knowledge / Memories / Peer Ask

```rust
let notes = client.list_knowledge(&[("search", "postgres")]).await?;
let note = client.get_knowledge("kb-1").await?;
client.create_knowledge(serde_json::json!({"title": "..."})).await?;
let memories = client.list_memories(&[("search", "pool")]).await?;
client.create_memory(serde_json::json!({"content": "..."})).await?;
client.delete_memory("mem-1").await?;
let asks = client.list_peer_asks(&[]).await?;
client.create_peer_ask(serde_json::json!({"question": "..."})).await?;
client.reply_peer_ask("ask-1", "Yes").await?;
client.cancel_peer_ask("ask-1").await?;
```

### Reference Data

```rust
let services = client.list_services(&[]).await?;
let on_call = client.who_is_on_call().await?;
let playbooks = client.get_playbooks("fp-abc").await?;
let secret = client.get_secret("secret-id").await?;
```

## Resilience

- **Idempotency**: `send_message` and `send_command` auto-inject an `Idempotency-Key` header (the only backend path that honors it). Retries replay from the backend cache, never re-execute.
- **REST retries**: transient failures (429, 500, 502, 503, 504, network) are retried up to `max_rest_retries` times with exponential backoff + jitter, honoring `Retry-After`. Non-replay-safe mutations execute exactly once.
- **Auth errors** (401/403) are terminal and never retried — retrieve via `take_fatal_error()`.
- **Envelope unwrap**: `{"data": ...}` responses are unwrapped automatically.

## Error Handling

```rust
use alga_agent_sdk::AlgaError;

match client.resolve_alert("fp-abc").await {
    Ok(_) => {}
    Err(AlgaError::Auth { status_code, message }) => {
        // 401/403 — token is invalid/revoked; do not retry.
    }
    Err(e @ AlgaError::Api { .. }) => {
        println!("retryable: {}", e.is_retryable());
    }
    Err(AlgaError::Connection(msg)) => {
        // Network failure.
    }
    Err(e) => { /* Request, Json, InvalidToken */ }
}
```

## License

MIT
