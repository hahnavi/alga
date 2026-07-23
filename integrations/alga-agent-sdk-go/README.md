# Alga Agent SDK for Go

A stdlib-only client library for building custom AI agents that integrate with
the [Alga](https://github.com/alga) SRE investigation platform.

## Installation

```bash
go get github.com/alga/agent-sdk-go
```

No external dependencies — stdlib only.

## What's new (modernization)

The SDK has been modernized to support the latest backend features and to be
production-grade out of the box:

- **Coordination tasks**: `DispatchTask`, `ClaimTask`, `CompleteTask`,
  `SynthesizeFindings`, `PostInvestigationThreadMessage` builders mirror the
  backend's commander/responder/communicator task subsystem. The
  `post_handoff` flow is deprecated in favor of dispatch + complete.
- **Idempotency-Key auto-injection**: every state-changing REST call gets a
  stable Idempotency-Key so a retry of the same logical call replays the
  cached response rather than double-firing the mutation. Callers driving
  their own outbox can pass an explicit key via `SendCommandWithKey`.
- **REST retries with exponential backoff**: transient failures (429, 5xx,
  network) are retried automatically. Honors `Retry-After`. Auth errors fail
  fast.
- **Structured logger**: inject `slog`-backed loggers via `WithLogger`. The
  legacy `Logf` shim is retained for backward compatibility but deprecated.
- **Functional options**: `WithHTTPClient`, `WithDedup`, `WithUserAgent`,
  `WithMaxRESTRetries`, `WithHeartbeatInterval`.
- **CoordinationTaskEvent**: SSE now surfaces `coordination_task_dispatched`
  events via `OnCoordinationTask`.
- **Normalized list responses**: `AlertListResponse.All()`,
  `InvestigationListResponse.All()`, `KnowledgeListResponse.All()` return the
  resources regardless of which JSON key the backend populated.
- **Truly stdlib-only**: the `google/uuid` dependency is gone. IDs are
  surfaced as opaque strings.

## Quick Start

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "syscall"

    alga "github.com/alga/agent-sdk-go"
)

func main() {
    serverURL := os.Getenv("ALGA_SERVER_URL")
    token := os.Getenv("ALGA_AGENT_TOKEN")

    client := alga.NewAlgaClient(serverURL, token,
        alga.WithLogger(alga.AsLogger(slog.Default().With("component", "alga"))),
        alga.WithMaxRESTRetries(3),
    )

    client.OnMessage = func(evt alga.MessageEvent) {
        // Every call gets an auto-injected Idempotency-Key so a transient
        // 503 retry is replayed from the backend cache rather than double-
        // delivered.
        _, _ = client.SendMessage(context.Background(), evt.ChatID, "Got it!", nil)
    }

    // Receive a coordination task from the incident commander.
    client.OnCoordinationTask = func(evt alga.CoordinationTaskEvent) {
        // Claim → do the work → complete.
        _, _ = client.SendCommand(context.Background(),
            "incident_coord_"+itoa(evt.IncidentNumber),
            alga.ClaimTask(evt.TaskID))
        // ... do the investigation/communication work ...
        _, _ = client.SendCommand(context.Background(),
            "incident_coord_"+itoa(evt.IncidentNumber),
            alga.CompleteTask(evt.TaskID, map[string]any{"finding": "leaky bucket"}))
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    if err := client.Connect(ctx); err != nil {
        panic(err)
    }

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
    client.Disconnect()
}

func itoa(n int64) string {
    if n == 0 {
        return "0"
    }
    var b [20]byte
    i := len(b)
    for n > 0 {
        i--
        b[i] = byte('0' + n%10)
        n /= 10
    }
    return string(b[i:])
}
```

## API Reference

### Client Construction

```go
client := alga.NewAlgaClient(serverURL, token,
    alga.WithHTTPClient(customHTTPClient),
    alga.WithLogger(structuredLogger),
    alga.WithDedup(alga.NewMessageDedup(5000, 10*time.Minute)),
    alga.WithUserAgent("my-agent/2.0"),
    alga.WithMaxRESTRetries(5),         // default 2; 0 disables retries
    alga.WithHeartbeatInterval(15*time.Second), // default 30s; floor 1s
)
```

### REST Methods

| Method | Description |
|--------|-------------|
| `ListAlerts(ctx, params)` | List alerts |
| `GetAlert(ctx, fingerprint)` | Get alert by fingerprint |
| `ResolveAlert(ctx, fingerprint)` | Resolve alert |
| `ReopenAlert(ctx, fingerprint)` | Reopen resolved alert |
| `ListInvestigations(ctx, params)` | List investigations |
| `GetInvestigation(ctx, id)` | Get investigation by ID |
| `PostUpdate(ctx, id, type, message)` | Post investigation update |
| `ListIncidentTasks(ctx, incidentNumber)` | List coordination tasks for an incident |
| `SendMessage(ctx, chatID, text, mentions)` | Send text message (auto Idempotency-Key) |
| `SendMessageWithKey(ctx, chatID, text, mentions, key)` | Send text with explicit Idempotency-Key |
| `SendCommand(ctx, chatID, cmd)` | Send investigation command (auto Idempotency-Key) |
| `SendCommandWithKey(ctx, chatID, cmd, key)` | Send command with explicit Idempotency-Key |
| `EditMessage(ctx, messageID, chatID, text)` | Edit message |
| `DeleteMessage(ctx, messageID, chatID)` | Delete message |
| `SendTyping(ctx, chatID, active)` | Send typing indicator |
| `SendHeartbeat(ctx)` | Send heartbeat |
| `ListKnowledge(ctx, params)` | List knowledge notes |
| `CreateKnowledge(ctx, params)` | Create knowledge note |
| `ListMemories(ctx, params)` | List memories |
| `CreateMemory(ctx, params)` | Create memory |
| `GetMemory(ctx, id)` | Get memory |
| `DeleteMemory(ctx, id)` | Delete memory |
| `ListPeerAsks(ctx, params)` | List peer asks |
| `CreatePeerAsk(ctx, params)` | Create peer ask |
| `GetPeerAsk(ctx, id)` | Get peer ask |
| `ReplyPeerAsk(ctx, id, reply)` | Reply to peer ask |
| `CancelPeerAsk(ctx, id)` | Cancel peer ask |
| `GetIncident(ctx, id)` | Get incident |
| `AddIncidentTimeline(ctx, id, message, eventType)` | Add incident timeline entry |
| `ListServices(ctx)` | List services |
| `WhoIsOnCall(ctx)` | Get current on-call |
| `GetCapabilities(ctx)` | Get agent capability catalog |
| `GetPlaybooks(ctx, alertFingerprint)` | Get playbooks |
| `SendIncidentSummary(ctx, incidentID, text)` | Send incident summary |
| `UploadMedia(ctx, filePath)` | Upload media file |

### SSE Events

Register callbacks on the client before calling `Connect`:

| Callback | Event Type | Description |
|----------|-----------|-------------|
| `OnConnected` | `connected` | SSE connection established |
| `OnMessage` | `message` | Incoming chat message |
| `OnTyping` | `typing` | Typing indicator |
| `OnInvestigationCancel` | `investigation_cancel` | Investigation cancelled |
| `OnInvestigationPause` | `investigation_pause` | Investigation paused |
| `OnInvestigationResume` | `investigation_resume` | Investigation resumed |
| `OnPeerFinding` | `peer_finding` | Peer agent finding |
| `OnPeerAsk` | `peer_ask` | Peer ask request |
| `OnPeerReply` | `peer_reply` | Peer ask reply |
| `OnAgentPresence` | `agent_presence` | Agent online/offline |
| `OnCoordinationTask` | `coordination_task_dispatched` | Incident commander dispatched a task |

### Command Builders

| Builder | Op |
|---------|-----|
| `ResolveAlert(fp)` | `resolve_alert` |
| `ReopenAlert(fp)` | `reopen_alert` |
| `SetOutcome(rootCause, resolution)` | `set_outcome` |
| `CancelInvestigation(reason)` | `cancel_investigation` |
| `PauseInvestigation(reason)` | `pause_investigation` |
| `TriageFeedback(...)` | `triage_feedback` |
| `AssignInvestigation(targetAgentID)` | `assign_investigation` |
| `PromoteToIncident(title, severity, priority)` | `promote_to_incident` |
| `PostInvestigationThreadMessage(message, internal)` | `post_investigation_thread_message` |
| `SetIncidentPriority(incidentNumber, priority)` | `set_incident_priority` |
| `SetIncidentSeverity(incidentNumber, severity)` | `set_incident_severity` |
| `TriggerEscalation(incidentNumber)` | `trigger_escalation` |
| `MitigateIncident(incidentNumber, reason)` | `mitigate_incident` |
| `ResolveIncident(incidentNumber, reason)` | `resolve_incident` |
| `BeginTriage(incidentNumber)` | `begin_triage` |
| `PromoteIncident(incidentNumber)` | `promote_incident` |
| `AssignIncidentRole(incidentNumber, role, user, token, scope)` | `assign_incident_role` |
| `PostHandoff(incidentNumber, msg, audience, urgency)` *(deprecated)* | `post_handoff` |
| `PublishStatusUpdate(incidentNumber, msg, level)` | `publish_status_update` |
| `SetIncidentResolutionDocs(...)` | `set_incident_resolution_docs` |
| `DispatchTask(incidentNumber, kind, goal, role)` | `dispatch_task` |
| `DispatchTaskToAgent(incidentNumber, kind, goal, agentID)` | `dispatch_task` |
| `ClaimTask(taskID)` | `claim_task` |
| `CompleteTask(taskID, result)` | `complete_task` |
| `SynthesizeFindings(incidentNumber, summary, evidence)` | `synthesize_findings` |

Task kinds: `TaskKindInvestigate`, `TaskKindCommunicate`, `TaskKindVerify`,
`TaskKindMitigate`.

### Error Types

| Type | Retryable? | Description |
|------|------------|-------------|
| `*AlgaAuthError` | Never | 401/403 — token is invalid, revoked, or expired |
| `*AlgaAPIError` | `IsRetryable()` | 4xx/5xx with status, body, RetryAfter |
| `*AlgaConnectionError` | `IsRetryable()` | Transport (DNS/TCP/TLS) failure |

Use `alga.IsAuthError(err)` to detect a bad token; use
`alga.IsRetryableError(err)` to decide whether to retry.

## Configuration

| Environment Variable | Description |
|---------------------|-------------|
| `ALGA_SERVER_URL` | Alga server base URL (e.g. `http://localhost:8080`) |
| `ALGA_AGENT_TOKEN` | Agent bearer token |

## License

MIT
