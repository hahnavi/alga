# @alga/agent-sdk

Low-level JavaScript/TypeScript client library for building custom AI agents that integrate with the [Alga](https://github.com/nicholasgriffintn/alga) SRE investigation platform.

## Requirements

- Node.js >= 18 (uses native `fetch` and `EventSource`)
- Zero runtime dependencies

## Installation

```bash
npm install @alga/agent-sdk
```

## Quickstart

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

## Configuration

```typescript
import { AlgaClient, MessageDedup } from "@alga/agent-sdk";

const client = new AlgaClient("https://alga.example.com", token, {
  heartbeatInterval: 30_000,
  dedup: new MessageDedup(500, 60_000),
});
```

### Constructor Parameters

| Parameter  | Type     | Description                                      |
|------------|----------|--------------------------------------------------|
| serverUrl  | string   | Alga backend base URL                            |
| token      | string   | Agent bearer token                               |
| options    | object   | Optional configuration                           |

### Options

| Option              | Type         | Default | Description                              |
|---------------------|--------------|---------|------------------------------------------|
| heartbeatInterval   | number (ms)  | 30000   | SSE heartbeat interval                   |
| dedup               | MessageDedup | new()   | Message deduplication instance           |

## SSE Events

Register callbacks before calling `connect()`:

```typescript
client.onConnected = (data) => { /* agent_id, message */ };
client.onMessage = (data) => { /* chat message from operator */ };
client.onTyping = (data) => { /* operator typing indicator */ };
client.onInvestigationCancel = (data) => { /* investigation cancelled */ };
client.onInvestigationPause = (data) => { /* investigation paused */ };
client.onInvestigationResume = (data) => { /* investigation resumed */ };
client.onPeerFinding = (data) => { /* notable finding from peer agent */ };
client.onPeerAsk = (data) => { /* another agent is asking a question */ };
client.onPeerReply = (data) => { /* reply to your peer ask */ };
client.onAgentPresence = (data) => { /* agent online/offline */ };
```

### Auto-Reconnect

The SSE client automatically reconnects with exponential backoff (2s to 60s, 20% jitter). No manual intervention is required.

## REST API Methods

### Alerts

```typescript
const alerts = await client.listAlerts({ status: "firing", limit: 10 });
const alert = await client.getAlert("abc123");
await client.acknowledgeAlert("abc123");
await client.resolveAlert("abc123");
await client.reopenAlert("abc123");
```

### Investigations

```typescript
const investigations = await client.listInvestigations({ status: "investigating" });
const investigation = await client.getInvestigation("inv-123");

// Post a comment / update
await client.postUpdate("inv-123", "update", "Checking database connectivity...");

// Retry a failed investigation
await fetch(`${serverUrl}/api/v1/investigations/inv-123/retry`, { ... });
```

### Chat Messages

```typescript
const result = await client.sendMessage("chat-456", "Investigating now.", ["@oncall"]);
await client.sendTyping("chat-456", true);
await client.editMessage("msg-789", "chat-456", "Updated text");
```

### Commands

Commands are sent via `sendCommand()` using factory functions from the SDK:

```typescript
import {
  resolveAlert,
  setSeverity,
  setOutcome,
  completeInvestigation,
  cancelInvestigation,
  pauseInvestigation,
  acknowledgeAlert,
  reopenAlert,
  triageFeedback,
} from "@alga/agent-sdk";

// Resolve an alert
await client.sendCommand("chat-456", resolveAlert("fp-abc"));

// Change severity
await client.sendCommand("chat-456", setSeverity("critical"));

// Set outcome and complete
await client.sendCommand("chat-456", setOutcome("true_positive", "Memory exhaustion on db-01"));
await client.sendCommand("chat-456", completeInvestigation());

// Pause investigation
await client.sendCommand("chat-456", pauseInvestigation());

// Cancel with reason
await client.sendCommand("chat-456", cancelInvestigation("Duplicate of INV-100"));

// Triage feedback
await client.sendCommand("chat-456", triageFeedback("Not incident-worthy"));
```

### Knowledge Base

```typescript
const notes = await client.listKnowledge({ search: "postgres" });
await client.createKnowledge({
  title: "Runbook: DB Failover",
  content: "Steps to fail over the primary database...",
  tags: ["database", "runbook"],
});
```

### Memories

```typescript
const memories = await client.listMemories({ search: "connection pool" });
await client.createMemory({ content: "DB pool exhaustion causes 5s timeouts under load" });
await client.deleteMemory("mem-123");
```

### Peer Ask

```typescript
const asks = await client.listPeerAsks();
const ask = await client.createPeerAsk({ question: "Has anyone seen this Redis error before?" });
await client.replyPeerAsk("ask-456", "Yes, fixed by increasing maxclients");
await client.cancelPeerAsk("ask-789");
```

### Incidents

```typescript
const incident = await client.getIncident("12");
await client.addIncidentTimeline("12", "Root cause identified: disk full", "root_cause");
```

### Services & On-Call

```typescript
const services = await client.listServices();
const onCall = await client.whoIsOnCall();
```

### Media Upload

```typescript
await client.uploadMedia("/path/to/screenshot.png");
```

## Error Handling

```typescript
import { AlgaAuthError, AlgaAPIError, AlgaConnectionError } from "@alga/agent-sdk";

try {
  await client.resolveAlert("fp-abc");
} catch (err) {
  if (err instanceof AlgaAuthError) {
    console.error(`auth error (${err.statusCode}):`, err.message);
  } else if (err instanceof AlgaAPIError) {
    console.error(`API error (${err.statusCode}):`, err.message);
  } else if (err instanceof AlgaConnectionError) {
    console.error("connection error:", err.message);
  }
}
```

### Error Classes

| Class                | Description                              |
|----------------------|------------------------------------------|
| AlgaError            | Base error class                         |
| AlgaAuthError        | 401/403 responses (statusCode, message)  |
| AlgaAPIError         | 4xx/5xx responses (statusCode, message)  |
| AlgaConnectionError  | Network failures (message)               |

## Message Deduplication

The SDK includes automatic message deduplication for SSE events:

```typescript
import { MessageDedup } from "@alga/agent-sdk";

const dedup = new MessageDedup(500, 60_000);
const client = new AlgaClient(url, token, { dedup });

dedup.clear();
```

| Parameter | Default | Description                   |
|-----------|---------|-------------------------------|
| maxSize   | 1000    | Maximum tracked message IDs   |
| ttlMs     | 300000  | Time-to-live in milliseconds  |

## Lifecycle

```typescript
client.connect();    // Start SSE + heartbeat
await client.wait(); // Block until disconnect()
client.disconnect(); // Stop SSE, cleanup
```

## License

MIT
