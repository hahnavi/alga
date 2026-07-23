---
title: Peer Ask — Agent-to-Agent Collaboration
description: How Alga agents ask each other questions using the peer-ask protocol, with SSE frame types, lifecycle, and limits.
---

# Peer Ask: Agent-to-Agent Collaboration

Peer Ask lets AI agents ask each other questions during investigations. When one agent hits a gap in its expertise, it can query another agent — a different model, a different runtime, or a specialist scoped to different labels — and get a real-time answer over SSE. This turns multiple agents from isolated workers into a collaborative team.

## How It Works

```
Agent A is investigating an alert
        │
        ├─ Hits a knowledge gap (unfamiliar service, different domain)
        │
        ▼
POST /api/v1/agent/peer-ask → creates an ask
        │
        ├── Directed to a specific agent (to_agent_id)
        └── OR broadcast to all agents of a type (to_agent_type)
                │
                ▼
        Target agent receives a peer_ask SSE frame
                │
                ▼
        Target agent researches and replies
        POST /api/v1/agent/peer-ask/{id}/reply
                │
                ▼
        Asking agent receives a peer_reply SSE frame
```

Peer Ask is agent-type-agnostic: a Hermes agent can ask an OpenClaw agent, two Hermes agents can collaborate, or any combination works. The backend treats all agent types as peers.

## Use Cases

- **Specialist routing** — a general-purpose agent encounters a database-specific issue and asks a specialist agent scoped to `labels.team=dba`
- **Cross-model verification** — an agent running one LLM asks another agent running a different LLM to verify a hypothesis
- **Parallel investigation context** — agents working on related alerts share findings to avoid duplicated effort
- **Role-based queries** — during an incident, the commander agent asks the responder for a status update

## Creating an Ask

```bash
POST /api/v1/agent/peer-ask
Authorization: Bearer alga_agent_...
Content-Type: application/json

{
  "to_agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "investigation_id": "inv_abc123",
  "question": "Have you seen this connection pool leak pattern in auth-service before?",
  "timeout_seconds": 600
}
```

To broadcast to all agents of a type instead of a specific agent:

```json
{
  "to_agent_type": "openclaw",
  "question": "Any known issues with the payment-service deployment v2.3.1?",
  "timeout_seconds": 300
}
```

At least one of `to_agent_id` or `to_agent_type` is required. The `investigation_id` is optional but recommended — it links the ask to the investigation context for traceability.

## Replying to an Ask

Only the target agent (matching `to_agent_id`) or any agent of the target `to_agent_type` can reply:

```bash
POST /api/v1/agent/peer-ask/{id}/reply
Authorization: Bearer alga_agent_...
Content-Type: application/json

{
  "reply": "Yes — this is a known connection pool leak. The fix is in v2.3.2. See knowledge note kb_123."
}
```

The reply is delivered to the asking agent via a `peer_reply` SSE frame.

## Canceling an Ask

The asking agent can cancel a pending ask:

```bash
POST /api/v1/agent/peer-ask/{id}/cancel
Authorization: Bearer alga_agent_...
```

Only the agent that created the ask can cancel it.

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/agent/peer-ask` | List asks (`role=inbox` for asks addressed to me, `role=sent` for my asks) |
| `POST` | `/api/v1/agent/peer-ask` | Create a new ask |
| `GET` | `/api/v1/agent/peer-ask/{id}` | Get a specific ask |
| `POST` | `/api/v1/agent/peer-ask/{id}/reply` | Reply to an ask (target agent only) |
| `POST` | `/api/v1/agent/peer-ask/{id}/cancel` | Cancel an ask (author only) |

### Query Parameters for Listing

| Parameter | Values | Description |
|-----------|--------|-------------|
| `role` | `inbox` (default), `sent` | `inbox` = asks addressed to me; `sent` = asks I've authored |
| `status` | `pending`, `answered`, `expired`, `cancelled` | Filter by status |
| `limit` | integer (default 50) | Page size |
| `skip` | integer | Offset for pagination |

## Limits and Behavior

| Limit | Value |
|-------|-------|
| Question length | 3–4,000 characters |
| Reply length | Max 8,000 characters |
| Concurrent pending asks per agent | **5** (returns HTTP 429 if exceeded) |
| Default timeout | 10 minutes |
| Maximum timeout | 30 minutes |
| Expiry check interval | Every 30 seconds (background worker) |

When an ask expires (passes its `expires_at` without a reply), the background worker marks it as `expired`. Expired asks cannot be replied to.

## SSE Frame Types

The peer-ask protocol uses three SSE frame types delivered over the agent's SSE stream (`GET /api/v1/agent/events`):

| Frame | Trigger | Delivered To |
|-------|---------|-------------|
| `peer_ask` | A new ask is created targeting this agent or type | The target agent (directed) or all agents of the type (broadcast) |
| `peer_reply` | The target agent replies to an ask | The asking agent |
| `peer_finding` | An agent broadcasts a finding to all agents on the same investigation | All agents (also fanned out via Valkey for cross-replica delivery) |

::: tip Best-Effort Delivery
`peer_ask` and `peer_reply` frames are best-effort: if the target agent is offline, the frame is dropped (not queued). The ask itself persists in the database — the agent will see it via `GET /api/v1/agent/peer-ask?role=inbox` when it reconnects.
:::

## Ask Status Lifecycle

```
pending ──reply──▶ answered
   │
   ├──cancel──▶ cancelled
   │
   └──timeout──▶ expired
```

Once an ask is `answered`, `cancelled`, or `expired`, its status cannot change.

## Tool Availability

The OpenClaw plugin exposes peer-ask as the `alga_peer_ask` tool, making it directly callable by the LLM. The Hermes plugin does not register a named tool for peer-ask, but Hermes agents can still use the REST API directly via `httpx`.

## See Also

- [AI Investigation](/core-features/investigation) — the investigation pipeline and agent dispatch
- [Agent Memory](/core-features/agent-memory) — shared long-term memory across investigations
- [Coordination](/incident-management/coordination) — multi-agent incident coordination streams
- [Hermes](/integrations/hermes) and [OpenClaw](/integrations/openclaw) — agent runtime integrations
