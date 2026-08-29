# Alga Agent SDK for Go

A stdlib-only client library for building custom AI agents that integrate with
the [Alga](https://github.com/alga) SRE investigation platform.

## Installation

```bash
go get github.com/alga/agent-sdk-go
```

No external dependencies — stdlib only.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
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
        // Only act on actionable deliveries; "observe" is context-only.
        if evt.Trigger == "observe" {
            return
        }
        _, _ = client.SendMessage(context.Background(), evt.ChatID, "Got it!", nil)
    }


    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    if err := client.Connect(ctx); err != nil {
        panic(err)
    }

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    select {
    case <-sigCh:
    case err := <-client.Err():
        // Terminal error (e.g. revoked token): the client has stopped
        // reconnecting. Obtain a valid token and Connect again.
        fmt.Fprintln(os.Stderr, "terminal error:", err)
    }
    client.Disconnect()
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

### Chat ID grammar

| Prefix                    | Thread                             |
| ------------------------- | ---------------------------------- |
| `alert_<number>`          | Alert investigation chat           |
| `incident_coord_<number>` | Incident coordination thread       |
| `incident_inv_<number>`   | Incident-scoped investigation chat |

### REST Methods

| Method                                                         | Description                                          |
| -------------------------------------------------------------- | ---------------------------------------------------- |
| `ListAlerts(ctx, params)`                                      | List alerts (returns `[]Alert`)                      |
| `GetAlert(ctx, fingerprint)`                                   | Get alert by fingerprint                             |
| `ResolveAlert(ctx, fingerprint)`                               | Resolve alert                                        |
| `ReopenAlert(ctx, fingerprint)`                                | Reopen resolved alert                                |
| `SendMessage(ctx, chatID, text, mentions)`                     | Send text message (auto Idempotency-Key)             |
| `SendMessageWithKey(ctx, chatID, text, mentions, key)`         | Send text with explicit Idempotency-Key              |
| `SendCommand(ctx, chatID, cmd)`                                | Send investigation command (auto Idempotency-Key)    |
| `SendCommandWithKey(ctx, chatID, cmd, key)`                    | Send command with explicit Idempotency-Key           |
| `SendIncidentSummary(ctx, incidentNumber, text)`               | Post `incident_summary` into the coordination thread |
| `SendDraft(ctx, chatID, draftID, text)`                        | Stream a partial (draft) message                     |
| `EditMessage(ctx, messageID, chatID, text)`                    | Edit message                                         |
| `DeleteMessage(ctx, messageID, chatID)`                        | Delete message                                       |
| `SendTyping(ctx, chatID, active)`                              | Send typing indicator                                |
| `SendHeartbeat(ctx)`                                           | Send heartbeat                                       |
| `GetIncident(ctx, incidentNumber)`                             | Get incident + role assignments                      |
| `GetIncidentTimeline(ctx, incidentNumber)`                     | Read incident timeline                               |
| `AddIncidentTimeline(ctx, incidentNumber, message, eventType)` | Add incident timeline entry                          |
| `UpdateIncidentSummary(ctx, incidentNumber, summary)`          | Patch incident summary                               |
| `ListKnowledge(ctx, params)`                                   | List knowledge notes                                 |
| `GetKnowledge(ctx, id)`                                        | Get knowledge note                                   |
| `CreateKnowledge(ctx, params)`                                 | Create knowledge note                                |
| `ListMemories(ctx, params)`                                    | List memories                                        |
| `CreateMemory(ctx, params)`                                    | Create memory                                        |
| `GetMemory(ctx, id)`                                           | Get memory                                           |
| `DeleteMemory(ctx, id)`                                        | Delete memory                                        |
| `ListPeerAsks(ctx, params)`                                    | List peer asks                                       |
| `CreatePeerAsk(ctx, params)`                                   | Create peer ask                                      |
| `GetPeerAsk(ctx, id)`                                          | Get peer ask                                         |
| `ReplyPeerAsk(ctx, id, reply)`                                 | Reply to peer ask                                    |
| `CancelPeerAsk(ctx, id)`                                       | Cancel peer ask                                      |
| `ListServices(ctx, params)`                                    | List services                                        |
| `WhoIsOnCall(ctx)`                                             | Get current on-call                                  |
| `GetPlaybooks(ctx, alertFingerprint)`                          | Get playbooks                                        |
| `GetSecret(ctx, secretID)`                                     | Fetch an allow-listed shared secret                  |

Responses use the backend's `{"data": ...}` envelope; the SDK unwraps it (and
falls back to flat bodies where the backend writes them).

### Idempotency & retries

The backend honors `Idempotency-Key` only on `POST /api/v1/agent/messages`.
The SDK auto-generates a key there, so transient failures (429, 5xx, network)
are retried safely — a replay returns the cached response instead of
double-delivering. `Retry-After` is honored; auth errors fail fast.

All other mutations have no replay cache and are executed **exactly once**:
they are never auto-retried, even with `WithMaxRESTRetries(n)`. GETs are
always retried up to the configured budget.

### SSE Events

Register callbacks on the client before calling `Connect`:

| Callback                | Event Type             | Description                                                                             |
| ----------------------- | ---------------------- | --------------------------------------------------------------------------------------- |
| `OnConnected`           | `connected`            | SSE connection established                                                              |
| `OnMessage`             | `message`              | Incoming chat message (`Trigger`: `dispatch`/`mention` = act, `observe` = context only) |
| `OnTyping`              | `typing`               | Typing indicator                                                                        |
| `OnInvestigationResume` | `investigation_resume` | Investigation resumed                                                                   |
| `OnPeerFinding`         | `peer_finding`         | Peer agent finding                                                                      |
| `OnPeerAsk`             | `peer_ask`             | Peer ask request                                                                        |
| `OnPeerReply`           | `peer_reply`           | Peer ask reply                                                                          |
| `OnSummarizeIncident`   | `summarize_incident`   | Backend requests an incident summary                                                    |
| `OnAlertAutoResolved`   | `alert_auto_resolved`  | An investigated alert auto-resolved                                                     |
| `OnIncidentCommsStale`  | `incident_comms_stale` | Incident comms went quiet past threshold                                                |
| `OnUnknownEvent`        | _(any other)_          | Raw hook for event types the SDK doesn't know                                           |

Terminal errors (revoked token, from either the SSE loop or the heartbeat
loop) arrive on `client.Err()`; after one arrives the client has stopped
reconnecting. Reconnect backoff is exponential 2s→60s with jitter and resets
after every successful connection.

### Command Builders

| Builder                                                                | Op                             |
| ---------------------------------------------------------------------- | ------------------------------ |
| `ResolveAlert(fp)`                                                     | `resolve_alert`                |
| `ReopenAlert(fp)`                                                      | `reopen_alert`                 |
| `SetOutcome(rootCause, resolution)`                                    | `set_outcome`                  |
| `CancelInvestigation(reason)`                                          | `cancel_investigation`         |
| `PauseInvestigation(reason)`                                           | `pause_investigation`          |
| `TriageFeedback(...)`                                                  | `triage_feedback`              |
| `AssignInvestigation(targetAgentID)`                                   | `assign_investigation`         |
| `PromoteToIncident(title, severity, priority)`                         | `promote_to_incident`          |
| `SetIncidentPriority(incidentNumber, priority)`                        | `set_incident_priority`        |
| `SetIncidentSeverity(incidentNumber, severity)`                        | `set_incident_severity`        |
| `TriggerEscalation(incidentNumber)`                                    | `trigger_escalation`           |
| `MitigateIncident(incidentNumber, reason)`                             | `mitigate_incident`            |
| `ResolveIncident(incidentNumber, reason)`                              | `resolve_incident`             |
| `BeginTriage(incidentNumber)`                                          | `begin_triage`                 |
| `PromoteIncident(incidentNumber)`                                      | `promote_incident`             |
| `AssignIncidentRoleToUser(incidentNumber, role, userID, scope)`        | `assign_incident_role`         |
| `AssignIncidentRoleToAgent(incidentNumber, role, agentTokenID, scope)` | `assign_incident_role`         |
| `PostHandoff(incidentNumber, msg, audience, urgency)`                  | `post_handoff`                 |
| `PublishStatusUpdate(incidentNumber, msg, level)`                      | `publish_status_update`        |
| `SetIncidentResolutionDocs(...)`                                       | `set_incident_resolution_docs` |

Command failures (unknown op, unauthorized chat, validation) surface as
`*AlgaAPIError` with status 404/422/500 and the backend outcome JSON in
`Message`.

### Error Types

| Type                   | Retryable?      | Description                                                          |
| ---------------------- | --------------- | -------------------------------------------------------------------- |
| `*AlgaAuthError`       | Never           | 401/403 — token is invalid, revoked, or expired                      |
| `*AlgaAPIError`        | `IsRetryable()` | 4xx/5xx with status, body, RetryAfter                                |
| `*AlgaConnectionError` | `IsRetryable()` | Transport failure; not retryable when caused by context cancellation |

Use `alga.IsAuthError(err)` to detect a bad token; use
`alga.IsRetryableError(err)` to decide whether to retry.

## Configuration

| Environment Variable | Description                                         |
| -------------------- | --------------------------------------------------- |
| `ALGA_SERVER_URL`    | Alga server base URL (e.g. `http://localhost:8080`) |
| `ALGA_AGENT_TOKEN`   | Agent bearer token                                  |

## License

MIT
