# @alga/agent-sdk

Low-level JavaScript/TypeScript client library for building custom AI agents that integrate with the [Alga](https://github.com/nicholasgriffintn/alga) SRE investigation platform. Zero runtime dependencies (Node.js ≥ 18 native `fetch`).

## Installation

```bash
npm install @alga/agent-sdk
```

## Quickstart

```typescript
import { AlgaClient, resolveAlert, setOutcome, dispatchTask } from "@alga/agent-sdk";

const client = new AlgaClient("https://alga.example.com", process.env.ALGA_AGENT_TOKEN!);

client.onMessage = (msg) => {
  console.log("received message:", msg.text);
};

// Terminal errors (e.g. revoked token) surface here once, then the client stops.
client.onErr((err) => console.error("terminal:", err.message));

client.connect();
```

## Configuration

```typescript
import { AlgaClient, MessageDedup } from "@alga/agent-sdk";

const client = new AlgaClient("https://alga.example.com", token, {
  heartbeatIntervalMs: 30_000,
  dedup: new MessageDedup(1000, 300_000),
  maxRestRetries: 2,
  userAgent: "my-agent/1.0",
});
```

### Options

| Option              | Type         | Default             | Description                                     |
| ------------------- | ------------ | ------------------- | ----------------------------------------------- |
| heartbeatIntervalMs | number (ms)  | 30000               | Heartbeat cadence (floor 1s)                    |
| dedup               | MessageDedup | new(1000, 300000)   | SSE message dedup cache                         |
| maxRestRetries      | number       | 2                   | Max REST retry attempts on transient errors     |
| userAgent           | string       | "alga-agent-sdk-js" | User-Agent header                               |
| fetchImpl           | typeof fetch | global fetch        | Custom fetch (for testing or non-Node runtimes) |

## SSE Events

Register callbacks before calling `connect()`. The SSE client auto-reconnects with exponential backoff (2s → 60s with jitter), honoring `Retry-After` on 429s.

| Callback                | Event type             | Description                              |
| ----------------------- | ---------------------- | ---------------------------------------- |
| `onConnected`           | `connected`            | Initial connection handshake             |
| `onMessage`             | `message`              | Chat message from operator/peer          |
| `onTyping`              | `typing`               | Typing indicator                         |
| `onInvestigationResume` | `investigation_resume` | Investigation resumed                    |
| `onPeerFinding`         | `peer_finding`         | Notable finding from a peer agent        |
| `onPeerAsk`             | `peer_ask`             | Another agent is asking a question       |
| `onPeerReply`           | `peer_reply`           | Reply to your peer ask                   |
| `onSummarizeIncident`   | `summarize_incident`   | Backend requests an incident summary     |
| `onAlertAutoResolved`   | `alert_auto_resolved`  | An investigated alert auto-resolved      |
| `onIncidentCommsStale`  | `incident_comms_stale` | Incident comms went quiet past threshold |
| `onUnknownEvent`        | any other              | Escape hatch for new backend event types |

## Commands

Commands are sent via `sendCommand()` using factory functions. All incident-scoped commands take `incidentNumber: number`.

```typescript
import {
  resolveAlert,
  reopenAlert,
  setOutcome,
  cancelInvestigation,
  pauseInvestigation,
  triageFeedback,
  assignInvestigation,
  promoteToIncident,
  setIncidentPriority,
  setIncidentSeverity,
  triggerEscalation,
  mitigateIncident,
  resolveIncident,
  beginTriage,
  promoteIncident,
  assignIncidentRoleToUser,
  assignIncidentRoleToAgent,
  postHandoff,
  publishStatusUpdate,
  setIncidentResolutionDocs,
} from "@alga/agent-sdk";

await client.sendCommand("alert_42", resolveAlert("fp-abc"));
await client.sendCommand("alert_42", setOutcome("Memory exhaustion on db-01", undefined));
await client.sendCommand("incident_coord_9", setIncidentPriority(9, "P1"));
await client.sendCommand(
  "incident_coord_9",
  dispatchTask(9, "investigate", "Find root cause", "responder"),
);
await client.sendCommand(
  "incident_coord_9",
  completeTask("task-uuid", { summary: "OOM in worker pool" }),
);
```

## REST Methods

### Alerts

```typescript
const alerts = await client.listAlerts({ status: "firing", limit: "10" });
const alert = await client.getAlert("fp-abc");
await client.resolveAlert("fp-abc");
await client.reopenAlert("fp-abc");
```

### Incidents

```typescript
const ctx = await client.getIncident(9); // IncidentContext (incident + roles)
const timeline = await client.getIncidentTimeline(9);
await client.addIncidentTimeline(9, "Root cause: disk full", "root_cause");
await client.updateIncidentSummary(9, "Summarized...");
```

### Messages

```typescript
const result = await client.sendMessage("chat-1", "Investigating.", ["@oncall"]);
await client.sendMessageWithKey("chat-1", "text", [], "my-key"); // explicit outbox key
await client.sendDraft("chat-1", "draft-1", "partial...");
await client.sendTyping("chat-1", true);
await client.editMessage("msg-9", "chat-1", "edited");
await client.deleteMessage("msg-9", "chat-1");
```

### Knowledge / Memories / Peer Ask

```typescript
const notes = await client.listKnowledge({ search: "postgres" });
const note = await client.getKnowledge("kb-1");
await client.createKnowledge({ title: "...", source_investigation_id: "...", confidence: 0.9 });
const memories = await client.listMemories({ search: "pool" });
await client.createMemory({ content: "..." });
await client.deleteMemory("mem-1");
const asks = await client.listPeerAsks();
await client.createPeerAsk({ question: "..." });
await client.replyPeerAsk("ask-1", "Yes");
await client.cancelPeerAsk("ask-1");
```

### Reference Data

```typescript
const services = await client.listServices();
const onCall = await client.whoIsOnCall();
const playbooks = await client.getPlaybooks("fp-abc");
const secret = await client.getSecret("secret-id");
```

## Resilience

- **Idempotency**: `sendMessage`, `sendCommand`, and their `WithKey` variants auto-inject an `Idempotency-Key` header (the only backend path that honors it). Retries of the same logical call replay from the backend cache, never re-execute.
- **REST retries**: transient failures (429, 500, 502, 503, 504, network) are retried up to `maxRestRetries` times with exponential backoff + jitter, honoring `Retry-After`. Non-replay-safe mutations execute exactly once.
- **Auth errors** (401/403) are terminal and never retried — they surface via `onErr()`.
- **Envelope unwrap**: `{"data": ...}` responses are unwrapped automatically.

## Error Handling

```typescript
import {
  AlgaAuthError,
  AlgaAPIError,
  AlgaConnectionError,
  isAuthError,
  isRetryableError,
} from "@alga/agent-sdk";

try {
  await client.resolveAlert("fp-abc");
} catch (err) {
  if (err instanceof AlgaAuthError) {
    // 401/403 — token is invalid/revoked; do not retry.
  } else if (err instanceof AlgaAPIError) {
    console.log(err.statusCode, err.retryAfterMs, err.isRetryable());
  } else if (err instanceof AlgaConnectionError) {
    // Network failure.
  }
}
```

## Lifecycle

```typescript
client.connect(); // Start SSE + heartbeat loops
client.disconnect(); // Stop loops and cleanup
```

## License

MIT
