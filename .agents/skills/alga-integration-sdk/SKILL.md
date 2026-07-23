---
name: alga-integration-sdk
description: Use when developing Alga agent SDKs, Hermes/OpenClaw integrations, bearer-token agent APIs, SSE clients, command messages, or SDK examples.
priority: P2
tags: [sdk, agent, integration, go, typescript, python, rust]
---

# Alga Agent SDK and Integration Development

Use this for agent-facing APIs, Hermes/OpenClaw integrations, SSE clients, bearer-token flows, command messages, and SDK examples. Do not assume SDK directories exist; inspect `integrations/` in the current checkout before planning changes.

## Check First

- Registered routes: `apps/backend/api/http.go`.
- Agent REST handlers: `apps/backend/api/agent_http.go`.
- Agent commands/messages: `apps/backend/api/agent_http.go`.
- Agent tool execution: `apps/backend/api/agent_tools.go`.
- Agent SSE: `apps/backend/api/agent_sse.go`.
- Peer ask: `apps/backend/api/peerask.go`.
- Bearer auth and token validation: `apps/backend/api/auth.go`.
- Current integrations: `integrations/`.
- Existing SDKs/examples only if present in this checkout.

Current checkouts may include Go, JS, Python, Rust, Hermes, OpenClaw, Mattermost, or Slack integration directories. Inspect the touched package manifest before assuming commands, generated files, or release layout.

## Auth and Route Rules

- Agent endpoints use `Authorization: Bearer <agent-token>`.
- Agent routes are registered with `agentBearerMiddleware(agentRateLimitMiddleware(...))`.
- Agent routes do not use CSRF cookies.
- SSE may support token query parameters only where backend code explicitly supports it.
- Store agent tokens as HMAC hashes through existing token paths; newly created bearer tokens are shown once.
- Use constant-time comparison in token validation paths.

## Endpoint Policy

- Derive current endpoint lists from `apps/backend/api/http.go`; do not trust copied inventories.
- Verify handler behavior in agent API files and tests before adding SDK methods.
- If changing backend endpoints, update backend tests first, then SDK clients/examples that exist in this checkout.
- Keep SDK method names stable and typed; expose useful error information for 401, 403, 404, 409, and 429 where relevant.

## SDK Requirements

When an SDK exists or is added, it should support:

- Bearer-auth REST requests with JSON content type.
- SSE connect with cancellation, reconnect/backoff, and typed event handlers.
- Heartbeat support; backend presence defaults come from backend config.
- Message sending for text and `inv_tool` payloads.
- URL escaping for path segments.
- 429 handling/backoff for rate limits.
- Tests or examples for auth failure, rate limit, reconnect, and command payloads where feasible.

## Message Shape

Text message example:

```json
{
  "chat_id": "investigation-uuid",
  "kind": "text",
  "text": "Analysis complete."
}
```

Command messages use `kind: "inv_tool"` and a command object. Verify supported operations in `apps/backend/api/agent_http.go` and `apps/backend/api/agent_tools.go` before adding helpers.

## Change Workflow

- Update backend endpoint/handler and tests when backend behavior changes.
- Update the Go SDK first only if a Go SDK exists in `integrations/` or the task asks to create one.
- Mirror changes across JS, Python, Rust, or plugin integrations only when those directories exist and the capability is cross-language.
- Update examples and docs near the touched integration.
- Do not add SDK directories solely because older guidance listed them.

## Verify

```bash
cd apps/backend
go test ./api/...
```

Then inspect each touched integration for its own manifest before running tests:

- Go: `go.mod`, then `go test ./...`.
- JS/OpenClaw: `package.json`, then package scripts such as `pnpm test`, `npm test`, or `pnpm build`.
- Python: `pyproject.toml` or `requirements.txt`, then the package's test command.
- Rust: `Cargo.toml`, then `cargo test`.

Only run commands for packages that exist in the current checkout.
