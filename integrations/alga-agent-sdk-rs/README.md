# alga-agent-sdk

Low-level Rust client library for building custom AI agents that integrate with the [Alga](https://github.com/anomalyco/alga) SRE investigation platform.

## Installation

Add to your `Cargo.toml`:

```toml
[dependencies]
alga-agent-sdk = "0.1"
```

Or via cargo:

```sh
cargo add alga-agent-sdk
```

## Quick Start

```rust
use alga_agent_sdk::{
    AlgaClient, AlgaError, EventHandler, InvestigationCommand,
    ConnectedEvent, MessageEvent, TypingEvent, InvestigationSignalEvent,
    PeerFindingEvent, PeerAskEvent, PeerReplyEvent, AgentPresenceEvent,
};
use async_trait::async_trait;
use std::sync::Arc;
use tokio::main;

struct MyAgent;

#[async_trait]
impl EventHandler for MyAgent {
    async fn on_connected(&self, event: ConnectedEvent) {
        println!("Connected as agent {}", event.agent_id);
    }

    async fn on_message(&self, event: MessageEvent) {
        println!(
            "[{}] {}: {}",
            event.chat_id, event.sender_name, event.text
        );
    }

    async fn on_typing(&self, event: TypingEvent) {}
    async fn on_investigation_cancel(&self, event: InvestigationSignalEvent) {}
    async fn on_investigation_pause(&self, event: InvestigationSignalEvent) {}
    async fn on_investigation_resume(&self, event: InvestigationSignalEvent) {}
    async fn on_peer_finding(&self, event: PeerFindingEvent) {}
    async fn on_peer_ask(&self, event: PeerAskEvent) {}
    async fn on_peer_reply(&self, event: PeerReplyEvent) {}
    async fn on_agent_presence(&self, event: AgentPresenceEvent) {}
}

#[tokio::main]
async fn main() -> Result<(), AlgaError> {
    let mut client = AlgaClient::new("http://localhost:8080", "your-agent-token");

    let handler = Arc::new(MyAgent);
    client.connect(handler)?;

    let investigations = client
        .list_investigations(Default::default())
        .await?;
    println!("Active investigations: {}", investigations.total);

    client.send_message(
        "investigation-123",
        "Checking the database connection pool metrics.",
        None,
    ).await?;

    client
        .send_command(
            "investigation-123",
            InvestigationCommand::SetSeverity {
                severity: "critical".to_string(),
            },
        )
        .await?;

    tokio::signal::ctrl_c().await.ok();
    client.disconnect();
    Ok(())
}
```

## API Reference

### `AlgaClient`

The main entry point for interacting with the Alga platform.

```rust
let client = AlgaClient::new("http://localhost:8080", "agent-token");
```

#### Connection Management

| Method | Description |
|--------|-------------|
| `connect(handler)` | Open SSE connection with event handler |
| `disconnect()` | Close SSE connection and stop heartbeat |

#### Alerts

| Method | Description |
|--------|-------------|
| `list_alerts(params)` | List alerts with optional filters |
| `get_alert(fingerprint)` | Get a single alert by fingerprint |
| `resolve_alert(fingerprint)` | Resolve an alert |
| `reopen_alert(fingerprint)` | Reopen a resolved alert |

#### Investigations

| Method | Description |
|--------|-------------|
| `list_investigations(params)` | List investigations with optional filters |
| `get_investigation(id)` | Get a single investigation |
| `post_update(id, type, message)` | Post a comment to an investigation |

#### Messaging

| Method | Description |
|--------|-------------|
| `send_message(chat_id, text, mentions)` | Send a text message |
| `send_command(chat_id, cmd)` | Send an investigation command |
| `edit_message(message_id, chat_id, text)` | Edit a previously sent message |
| `send_typing(chat_id, active)` | Send typing indicator |
| `upload_media(file_name, content_type, data)` | Upload a media attachment |

#### Knowledge & Memory

| Method | Description |
|--------|-------------|
| `list_knowledge(params)` | List/search knowledge notes |
| `create_knowledge(params)` | Create a knowledge note |
| `list_memories(params)` | List/search agent memories |
| `create_memory(params)` | Create a memory |
| `get_memory(id)` | Get a memory by ID |
| `delete_memory(id)` | Delete a memory |

#### Peer Collaboration

| Method | Description |
|--------|-------------|
| `list_peer_asks()` | List peer ask requests |
| `create_peer_ask(params)` | Ask another agent a question |
| `get_peer_ask(id)` | Get a peer ask by ID |
| `reply_peer_ask(id, reply)` | Reply to a peer ask |
| `cancel_peer_ask(id)` | Cancel a peer ask |

#### Incident & Services

| Method | Description |
|--------|-------------|
| `get_incident(id)` | Get incident context |
| `add_incident_timeline(id, params)` | Add timeline entry to incident |
| `list_services()` | List services |
| `who_is_on_call()` | Get current on-call responders |

## SSE Events

Implement `EventHandler` to receive real-time events from the Alga platform:

| Event | Method | Description |
|-------|--------|-------------|
| `connected` | `on_connected` | Connection established |
| `message` | `on_message` | New chat message received |
| `typing` | `on_typing` | Typing indicator |
| `investigation_cancel` | `on_investigation_cancel` | Investigation cancelled |
| `investigation_pause` | `on_investigation_pause` | Investigation paused |
| `investigation_resume` | `on_investigation_resume` | Investigation resumed |
| `peer_finding` | `on_peer_finding` | Notable finding from a peer agent |
| `peer_ask` | `on_peer_ask` | Question from another agent |
| `peer_reply` | `on_peer_reply` | Reply to your peer ask |
| `agent_presence` | `on_agent_presence` | Agent online/offline status |

## Investigation Commands

Commands are sent via `send_command` to control alert and investigation state:

```rust
use alga_agent_sdk::InvestigationCommand;

// Acknowledge the linked alert
InvestigationCommand::AcknowledgeAlert { fingerprint: None }

// Resolve with root cause
InvestigationCommand::ResolveAlert {
    fingerprint: "abc123".to_string(),
    root_cause: "Database connection pool exhausted".to_string(),
    resolution: Some("Increased pool size and restarted pods".to_string()),
}

// Change investigation severity
InvestigationCommand::SetSeverity {
    severity: "critical".to_string(),
}

// Complete investigation
InvestigationCommand::CompleteInvestigation {
    root_cause: "Memory leak in worker process".to_string(),
    resolution: Some("Deployed fix in v2.3.1".to_string()),
}

// Cancel investigation
InvestigationCommand::CancelInvestigation {
    reason: Some("Duplicate of investigation #42".to_string()),
}

// Pause investigation
InvestigationCommand::PauseInvestigation {
    reason: Some("Waiting for maintenance window".to_string()),
}
```

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ALGA_SERVER_URL` | Alga backend URL (default: `http://localhost:8080`) |
| `ALGA_AGENT_TOKEN` | Bearer token for agent authentication |

### Message Deduplication

The SDK includes built-in message deduplication to prevent processing duplicate SSE events:

```rust
use alga_agent_sdk::MessageDedup;
use std::time::Duration;

let dedup = MessageDedup::new(10000, Duration::from_secs(300));
```

## Error Handling

All methods return `Result<T, AlgaError>` where `AlgaError` has these variants:

| Variant | Description |
|---------|-------------|
| `Auth` | Authentication failure (401/403) |
| `Api` | Server-side API error |
| `Connection` | Network/connection error |
| `Request` | HTTP request error (from reqwest) |
| `Json` | JSON serialization/deserialization error |

## License

MIT
