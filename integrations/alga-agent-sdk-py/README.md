# alga-agent-sdk

Low-level async Python client library for building custom AI agents that integrate with the [Alga](https://github.com/anomalyco/alga) SRE investigation platform. Built on `asyncio` + `httpx` + `pydantic` v2.

## Installation

```bash
pip install -e integrations/alga-agent-sdk-py
```

Requires Python >= 3.10. Dependencies (`httpx>=0.27`, `pydantic>=2.0`) are installed automatically.

## Quickstart

```python
import asyncio
from alga_agent_sdk import AlgaClient, resolve_alert, set_outcome

client = AlgaClient("https://alga.example.com", token="your-agent-bearer-token")


async def on_message(msg):
    print("received message:", msg.text)


async def on_coordination_task(evt):
    print("task dispatched:", evt.goal)


async def on_err(err):
    print("terminal:", err)


client.on_message = on_message
client.on_coordination_task = on_coordination_task
client.on_err(on_err)


async def main():
    await client.connect()
    try:
        await client.wait()
    finally:
        await client.disconnect()


asyncio.run(main())
```

## Configuration

```python
from alga_agent_sdk import AlgaClient, MessageDedup

client = AlgaClient(
    "https://alga.example.com",
    token,
    heartbeat_interval=30.0,
    dedup=MessageDedup(1000, 300.0),
    max_rest_retries=2,
    user_agent="my-agent/1.0",
)
```

### Options

| Option             | Type         | Default             | Description                                 |
| ------------------ | ------------ | ------------------- | ------------------------------------------- |
| heartbeat_interval | float (sec)  | 30.0                | Heartbeat cadence (floor 1s)                |
| dedup              | MessageDedup | new(1000, 300.0)    | SSE message dedup cache                     |
| max_rest_retries   | int          | 2                   | Max REST retry attempts on transient errors |
| user_agent         | str          | "alga-agent-sdk-py" | User-Agent header                           |

## SSE Events

Register callbacks before calling `connect()`. The SSE client auto-reconnects with exponential backoff (2s → 60s with ±20% jitter), honoring `Retry-After` on 429s.

| Callback                  | Event type                     | Description                              |
| ------------------------- | ------------------------------ | ---------------------------------------- |
| `on_connected`            | `connected`                    | Initial connection handshake             |
| `on_message`              | `message`                      | Chat message from operator/peer          |
| `on_typing`               | `typing`                       | Typing indicator                         |
| `on_investigation_resume` | `investigation_resume`         | Investigation resumed                    |
| `on_peer_finding`         | `peer_finding`                 | Notable finding from a peer agent        |
| `on_peer_ask`             | `peer_ask`                     | Another agent is asking a question       |
| `on_peer_reply`           | `peer_reply`                   | Reply to your peer ask                   |
| `on_coordination_task`    | `coordination_task_dispatched` | Commander dispatched a task to you       |
| `on_summarize_incident`   | `summarize_incident`           | Backend requests an incident summary     |
| `on_alert_auto_resolved`  | `alert_auto_resolved`          | An investigated alert auto-resolved      |
| `on_incident_comms_stale` | `incident_comms_stale`         | Incident comms went quiet past threshold |
| `on_unknown_event`        | any other                      | Escape hatch for new backend event types |

Callbacks may be plain functions or coroutines. Messages prefixed with 🔒 (U+1F512) are automatically filtered, and duplicates are deduplicated via `MessageDedup`.

## Commands

Commands are sent via `send_command()` using factory functions. All incident-scoped commands take `incident_number: int`.

```python
from alga_agent_sdk import (
    resolve_alert, reopen_alert, set_outcome, cancel_investigation, pause_investigation,
    triage_feedback, assign_investigation, promote_to_incident,
    set_incident_priority, set_incident_severity, trigger_escalation, mitigate_incident,
    resolve_incident, begin_triage, promote_incident,
    assign_incident_role_to_user, assign_incident_role_to_agent,
    post_handoff, publish_status_update, set_incident_resolution_docs,
)

await client.send_command("alert_42", resolve_alert("fp-abc"))
await client.send_command("alert_42", set_outcome("Memory exhaustion on db-01"))
await client.send_command("incident_coord_9", set_incident_priority(9, "P1"))
```

## REST Methods

### Alerts

```python
alerts = await client.list_alerts({"status": "firing", "limit": "10"})
alert = await client.get_alert("fp-abc")
await client.resolve_alert("fp-abc")
await client.reopen_alert("fp-abc")
```

### Incidents

```python
ctx = await client.get_incident(9)          # IncidentContext (incident + roles)
timeline = await client.get_incident_timeline(9)
await client.add_incident_timeline(9, "Root cause: disk full", "root_cause")
await client.update_incident_summary(9, "Summarized...")
```

### Messages

```python
result = await client.send_message("chat-1", "Investigating.", ["@oncall"])
await client.send_message_with_key("chat-1", "text", [], "my-key")  # explicit outbox key
await client.send_draft("chat-1", "draft-1", "partial...")
await client.send_typing("chat-1", True)
await client.edit_message("msg-9", "chat-1", "edited")
await client.delete_message("msg-9", "chat-1")
```

### Knowledge / Memories / Peer Ask

```python
notes = await client.list_knowledge({"search": "postgres"})
note = await client.get_knowledge("kb-1")
await client.create_knowledge({"title": "...", "source_investigation_id": "...", "confidence": 0.9})
memories = await client.list_memories({"search": "pool"})
await client.create_memory({"content": "..."})
await client.delete_memory("mem-1")
asks = await client.list_peer_asks()
await client.create_peer_ask({"question": "..."})
await client.reply_peer_ask("ask-1", "Yes")
await client.cancel_peer_ask("ask-1")
```

### Reference Data

```python
services = await client.list_services()
on_call = await client.who_is_on_call()
playbooks = await client.get_playbooks("fp-abc")
secret = await client.get_secret("secret-id")
```

## Resilience

- **Idempotency**: `send_message`, `send_command`, and their `_with_key` variants auto-inject an `Idempotency-Key` header (the only backend path that honors it). Retries of the same logical call replay from the backend cache, never re-execute.
- **REST retries**: transient failures (429, 500, 502, 503, 504, network) are retried up to `max_rest_retries` times with exponential backoff + jitter, honoring `Retry-After`. Non-replay-safe mutations execute exactly once.
- **Auth errors** (401/403) are terminal and never retried — they surface via `on_err()`.
- **Envelope unwrap**: `{"data": ...}` responses are unwrapped automatically.

## Error Handling

```python
from alga_agent_sdk import AlgaAuthError, AlgaAPIError, AlgaConnectionError, is_auth_error, is_retryable_error

try:
    await client.resolve_alert("fp-abc")
except AlgaAuthError as err:
    # 401/403 — token is invalid/revoked; do not retry.
    print(err.status_code, err.message)
except AlgaAPIError as err:
    print(err.status_code, err.retry_after, err.is_retryable())
except AlgaConnectionError as err:
    # Network failure.
    pass
```

## Lifecycle

```python
await client.connect()   # Start SSE + heartbeat loops
await client.wait()      # Block until loops exit
await client.disconnect()  # Stop loops and cleanup
```

## License

MIT
