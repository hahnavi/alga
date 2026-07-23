# Alga Agent SDK — Python

A low-level async client library for building custom AI agents that integrate with the [Alga](https://github.com/anomalyco/alga) SRE investigation platform.

## Installation

```bash
pip install -e integrations/alga-agent-sdk-py
```

Requires Python >=3.10. Dependencies (`httpx>=0.27`, `pydantic>=2.0`) are installed automatically.

## Quickstart

```python
import asyncio
from alga_agent_sdk import AlgaClient, set_severity, resolve_alert, complete_investigation

client = AlgaClient(
    server_url="http://localhost:8080",
    token="your-agent-bearer-token",
)

async def on_connected(evt):
    print(f"Connected as agent {evt.agent_id}")

async def on_message(evt):
    print(f"[{evt.sender_name}] {evt.text}")

    if evt.text.startswith("!resolve"):
        await client.send_command(
            evt.chat_id,
            resolve_alert("abc123fingerprint"),
        )

async def on_investigation_cancel(signal):
    print(f"Investigation {signal.investigation_id} cancelled: {signal.reason}")

async def on_peer_ask(evt):
    answer = await my_llm_answer(evt.question)
    await client.reply_peer_ask(evt.ask_id, answer)

async def main():
    client.on_connected = on_connected
    client.on_message = on_message
    client.on_investigation_cancel = on_investigation_cancel
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

## API Reference

### REST Methods

| Method | Signature | Returns |
|--------|-----------|---------|
| `list_alerts` | `(status?, severity?, search?, start_date?, end_date?, limit?, skip?)` | `AlertListResponse` |
| `get_alert` | `(fingerprint)` | `Alert` |
| `resolve_alert` | `(fingerprint)` | `Alert` |
| `reopen_alert` | `(fingerprint)` | `Alert` |
| `list_investigations` | `(status?, severity?, search?, limit?, skip?)` | `InvestigationListResponse` |
| `get_investigation` | `(investigation_id)` | `Investigation` |
| `post_update` | `(investigation_id, update_type, message)` | `Investigation` |
| `send_message` | `(chat_id, text, mentions?)` | `SendMessageResponse` |
| `send_command` | `(chat_id, command)` | `CommandResponse` |
| `edit_message` | `(message_id, chat_id, text)` | `None` |
| `delete_message` | `(message_id, chat_id)` | `None` |
| `send_typing` | `(chat_id, active=True)` | `None` |
| `send_heartbeat` | `()` | `None` |
| `list_knowledge` | `(query?, kind?, tag?, limit?, skip?)` | `KnowledgeListResponse` |
| `create_knowledge` | `(kind, title, body_markdown, tags?, source_investigation_id?, confidence?)` | `KnowledgeNote` |
| `list_memories` | `(query?, limit?, skip?)` | `MemoryListResponse` |
| `create_memory` | `(content, memory_type?, investigation_id?, correlation_key?, labels?, confidence?, expires_at?)` | `Memory` |
| `get_memory` | `(memory_id)` | `Memory` |
| `delete_memory` | `(memory_id)` | `None` |
| `list_peer_asks` | `(role="inbox", status?, limit?, skip?)` | `PeerAskListResponse` |
| `create_peer_ask` | `(question, to_agent_id?, to_agent_type?, investigation_id?, timeout_seconds=600)` | `PeerAsk` |
| `get_peer_ask` | `(ask_id)` | `PeerAsk` |
| `reply_peer_ask` | `(ask_id, reply)` | `PeerAsk` |
| `cancel_peer_ask` | `(ask_id)` | `None` |
| `get_incident` | `(incident_id)` | `Incident` |
| `add_incident_timeline` | `(incident_id, message, event_type?)` | `None` |
| `list_services` | `()` | `list[Service]` |
| `who_is_on_call` | `()` | `list[dict]` |
| `upload_media` | `(file_path)` | `dict` |

## Investigation Commands

Factory functions that produce `InvestigationCommand` objects for use with `send_command`:

| Function | Parameters |
|----------|------------|
| `resolve_alert` | `fingerprint` |
| `reopen_alert` | `fingerprint` |
| `set_severity` | `severity` |
| `set_outcome` | `root_cause?, resolution?` |
| `complete_investigation` | — |
| `cancel_investigation` | `reason?` |
| `pause_investigation` | `reason?` |
| `triage_feedback` | `is_incident`, `summary?` |

## SSE Events

Register callbacks on the `AlgaClient` instance before calling `connect()`:

| Callback | Event Type | Fields |
|----------|-----------|--------|
| `on_connected` | `ConnectedEvent` | `client_id`, `agent_id` |
| `on_message` | `MessageEvent` | `type`, `message_id`, `chat_id`, `text`, `sender_id`, `sender_name` |
| `on_typing` | `TypingEvent` | `type`, `chat_id`, `active` |
| `on_investigation_cancel` | `InvestigationSignalEvent` | `investigation_id`, `reason`, `actor` |
| `on_investigation_pause` | `InvestigationSignalEvent` | `investigation_id`, `reason`, `actor` |
| `on_investigation_resume` | `InvestigationSignalEvent` | `investigation_id`, `reason`, `actor` |
| `on_peer_finding` | `PeerFindingEvent` | `type`, `investigation_id`, `peer_agent_id`, `peer_agent_type`, `text`, `labels`, `created_at` |
| `on_peer_ask` | `PeerAskEvent` | `type`, `ask_id`, `from_agent_id`, `from_agent_name`, `from_agent_type`, `investigation_id`, `question`, `expires_at`, `created_at` |
| `on_peer_reply` | `PeerReplyEvent` | `type`, `ask_id`, `investigation_id`, `reply`, `replied_by_agent_id`, `replied_by_agent_name`, `answered_at` |
| `on_agent_presence` | `AgentPresenceEvent` | `agent_id`, `online` |

Messages prefixed with the lock emoji (🔒) are automatically filtered. Duplicate messages are deduplicated using `MessageDedup` (configurable, defaults to 1000 entries / 300s TTL).

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `server_url` | — | Alga backend URL (required) |
| `token` | — | Agent bearer token (required) |
| `heartbeat_interval` | `30.0` | Seconds between heartbeat POSTs |
| `dedup` | `MessageDedup()` | Custom dedup instance, or `None` to disable |

## Error Handling

All errors inherit from `AlgaError`:

| Error | When |
|-------|------|
| `AlgaAuthError` | 401/403 response — check your token |
| `AlgaAPIError` | Any other 4xx/5xx response |
| `AlgaConnectionError` | Network failure or calling methods before `connect()` |
