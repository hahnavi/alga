---
title: API Reference
description: Complete REST API reference for Alga — every endpoint, method, path, authentication type, and RBAC permission across alerts, incidents, agents, teams, on-call, and more.
---

# API Reference

All endpoints are relative to `http://your-alga-host:8080`. Operator/UI endpoints use session authentication via HTTP-only cookies; webhook endpoints use bearer tokens; agent endpoints use agent bearer tokens.

## Authentication

### Session Auth

Most operator endpoints require session authentication via HTTP-only cookies. Obtain a session by logging in. A CSRF token (`alga_csrf` cookie) must be sent back as the `X-CSRF-Token` header on every non-GET request.

```sh
# Login and save cookies
curl -c cookies.txt -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@alga.local", "password": "your-password"}'

# Use session for subsequent requests
curl -b cookies.txt http://localhost:8080/api/v1/alerts
```

### Personal Access Tokens

Personal Access Tokens (`alga_pat_...`) authenticate the same operator endpoints without cookies/CSRF. See [Personal Access Tokens](/operations/personal-access-tokens).

### Webhook Bearer Auth

Alert ingestion uses webhook bearer tokens (`alga_...`):

```sh
curl -X POST http://localhost:8080/webhooks/alerts \
  -H "Authorization: Bearer alga_..." \
  -H "Content-Type: application/json" \
  -d '{"alerts": [...]}'
```

### Agent Bearer Auth

Agent endpoints use agent bearer tokens (`alga_agent_...`):

```sh
# SSE stream
curl -N http://localhost:8080/api/v1/agent/events \
  -H "Authorization: Bearer alga_agent_..."

# REST API
curl http://localhost:8080/api/v1/agent/alerts \
  -H "Authorization: Bearer alga_agent_..."
```

## Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/auth/login` | None | Login with email and password |
| `POST` | `/api/v1/auth/logout` | Session | Logout and clear session |
| `GET` | `/api/v1/auth/me` | Session | Get current user info |
| `POST` | `/api/v1/auth/refresh` | Session | Refresh session token |
| `POST` | `/api/v1/auth/change-password` | Session | Change password |
| `POST` | `/api/v1/auth/change-email` | Session | Change email address |
| `POST` | `/api/v1/auth/profile` | Session | Update display name / profile |
| `POST` | `/api/v1/auth/forgot-password` | None | Request password reset email |
| `POST` | `/api/v1/auth/reset-password` | None | Reset password with token |
| `GET` | `/api/v1/auth/google/enabled` | None | Check if Google Sign-In is enabled |
| `GET` | `/api/v1/auth/google` | None | Start Google OAuth flow |
| `GET` | `/api/v1/auth/google/callback` | None | Google OAuth callback |
| `GET` | `/api/v1/auth/slack/enabled` | None | Check if Slack Sign-In is enabled |
| `GET` | `/api/v1/auth/slack` | None | Start Slack Sign-In OAuth flow |
| `GET` | `/api/v1/auth/slack/callback` | None | Slack Sign-In OAuth callback |
| `GET` | `/api/v1/auth/oidc/providers` | None | List enabled OIDC SSO providers |
| `GET` | `/api/v1/auth/oidc/{id}/authorize` | None | Start OIDC SSO flow |
| `GET` | `/api/v1/auth/oidc/{id}/callback` | None | OIDC SSO callback |

## Alerts

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/alerts` | Session | — | List alerts (query: status, channel, provider, severity, search, start_date, end_date, limit, skip) |
| `POST` | `/api/v1/alerts` | Session | — | Create manual alert |
| `GET` | `/api/v1/alerts/{alert_number}` | Session | — | Get alert by alert number |
| `GET` | `/api/v1/alerts/{alert_number}/related` | Session | `alerts:read` | Get related alerts and linked incident |
| `GET` | `/api/v1/alerts/{alert_number}/thread` | Session | `alerts:read` | Get alert discussion thread |
| `POST` | `/api/v1/alerts/{alert_number}/thread/typing` | Session | `alerts:write` | Send typing indicator to alert thread |
| `POST` | `/api/v1/alerts/{alert_number}/thread/messages` | Session | `alerts:write` | Add message to alert thread |
| `POST` | `/api/v1/alerts/{alert_number}/acknowledge` | Session | — | Acknowledge alert |
| `POST` | `/api/v1/alerts/{alert_number}/resolve` | Session | — | Resolve alert |
| `POST` | `/api/v1/alerts/{alert_number}/reopen` | Session | — | Reopen resolved alert |
| `POST` | `/api/v1/alerts/{alert_number}/investigate` | Session | — | Trigger AI investigation |
| `DELETE` | `/api/v1/alerts/{alert_number}` | Session | `alerts:delete` | Delete alert |

## Webhook Tokens

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/webhook-tokens` | Session | `tokens:manage` | List webhook tokens |
| `POST` | `/api/v1/webhook-tokens` | Session | `tokens:manage` | Create webhook token |
| `DELETE` | `/api/v1/webhook-tokens/{id}` | Session | `tokens:manage` | Revoke webhook token |

## Agent Tokens

Agent tokens are managed by operators; the resulting bearer token is used by agents on the [Agent REST API](#agent-rest-api) and [Agent SSE](#agent-sse).

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/agent-tokens` | Session | `tokens:manage` | List agent tokens (with online presence) |
| `POST` | `/api/v1/agent-tokens` | Session | `tokens:manage` | Create agent token |
| `PUT` | `/api/v1/agent-tokens/{id}` | Session | `tokens:manage` | Update agent config (enabled, scope, label_selectors) |
| `POST` | `/api/v1/agent-tokens/{id}/set-default` | Session | `tokens:manage` | Set default investigation agent |
| `POST` | `/api/v1/agent-tokens/{id}/enable` | Session | `tokens:manage` | Enable agent |
| `POST` | `/api/v1/agent-tokens/{id}/disable` | Session | `tokens:manage` | Disable agent |
| `POST` | `/api/v1/agent-tokens/{id}/regenerate` | Session | `tokens:manage` | Regenerate agent bearer token |
| `DELETE` | `/api/v1/agent-tokens/{id}` | Session | `tokens:manage` | Revoke agent token |
| `GET` | `/api/v1/agent/capabilities` | Session | `tokens:manage` | List available agent capabilities |

### Agent Direct Messages (Operator ↔ Agent)

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/agent-tokens/{id}/chat/messages` | Session | `tokens:manage` | Get agent DM history |
| `POST` | `/api/v1/agent-tokens/{id}/chat/messages` | Session | `tokens:manage` | Send message to agent |
| `POST` | `/api/v1/agent-tokens/{id}/chat/typing` | Session | `tokens:manage` | Send typing indicator |

## Agent REST API

All endpoints below require agent bearer token auth (`alga_agent_...`) and are rate-limited per agent.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agent/alerts` | List alerts |
| `GET` | `/api/v1/agent/alerts/{fp}` | Get alert by fingerprint |
| `GET` | `/api/v1/agent/alerts/{fp}/thread` | Get alert thread |
| `POST` | `/api/v1/agent/alerts/{fp}/thread/messages` | Add alert thread message |
| `POST` | `/api/v1/agent/alerts/{fp}/resolve` | Resolve alert |
| `POST` | `/api/v1/agent/alerts/{fp}/reopen` | Reopen alert |
| `GET` | `/api/v1/agent/knowledge` | List/search knowledge notes |
| `GET` | `/api/v1/agent/knowledge/{id}` | Get knowledge note |
| `GET` | `/api/v1/agent/memories` | List/search memories |
| `POST` | `/api/v1/agent/memories` | Create memory |
| `GET` | `/api/v1/agent/memories/{id}` | Get memory |
| `DELETE` | `/api/v1/agent/memories/{id}` | Delete memory |
| `GET` | `/api/v1/agent/secrets/{id}` | Fetch a shared secret the agent is authorized to read |
| `GET` | `/api/v1/agent/playbooks` | List playbooks |
| `GET` | `/api/v1/agent/services` | List services |
| `GET` | `/api/v1/agent/on-call/current` | Who is on call |
| `GET` | `/api/v1/agent/incidents/{incident_number}` | Get incident context (only if assigned) |
| `PATCH` | `/api/v1/agent/incidents/{incident_number}` | Update incident (priority, severity) |
| `GET` | `/api/v1/agent/incidents/{incident_number}/timeline` | Get incident timeline |
| `POST` | `/api/v1/agent/incidents/{incident_number}/timeline` | Add timeline entry |
| `GET` | `/api/v1/agent/incidents/{incident_number}/tasks` | List coordination tasks for incident |
| `POST` | `/api/v1/agent/messages` | Send a text message or invoke an agent tool (`kind: "text"` or `kind: "inv_tool"`) |
| `PUT` | `/api/v1/agent/messages/{id}` | Edit a previous message |
| `DELETE` | `/api/v1/agent/messages/{id}` | Delete a previously sent message |
| `GET`/`POST` | `/api/v1/agent/drafts` | Read or store an unsent draft |
| `POST` | `/api/v1/agent/typing` | Typing indicator |
| `POST` | `/api/v1/agent/heartbeat` | Renew SSE presence lease |
| `GET`/`POST` | `/api/v1/agent/peer-ask` | List peer asks / ask another agent for help |
| `GET` | `/api/v1/agent/peer-ask/{id}` | Get peer ask |
| `POST` | `/api/v1/agent/peer-ask/{id}/reply` | Reply to a peer ask |
| `POST` | `/api/v1/agent/peer-ask/{id}/cancel` | Cancel a peer ask |

::: tip Agent tools via messages
Agent capabilities like assigning roles, triggering escalation, publishing status updates, and promoting incidents are invoked by posting to `/api/v1/agent/messages` with `kind: "inv_tool"`. See the [Agent SDKs](/integrations/agent-sdks) for the command factory helpers.
:::

## Agent SSE

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/agent/events` | Agent Bearer | SSE stream for agents (investigation dispatch, chat, peer findings) |

## Routes

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/routes` | Session | — | Get routing rules and default destinations |
| `PUT` | `/api/v1/routes` | Session | `routes:write` | Save routing rules |

## Maintenance Windows

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/maintenance-windows` | Session | — | List maintenance windows |
| `POST` | `/api/v1/maintenance-windows` | Session | `routes:write` | Create maintenance window |
| `GET` | `/api/v1/maintenance-windows/{id}` | Session | — | Get maintenance window |
| `PUT` | `/api/v1/maintenance-windows/{id}` | Session | `routes:write` | Update maintenance window |
| `DELETE` | `/api/v1/maintenance-windows/{id}` | Session | `routes:write` | Delete maintenance window |

## Integrations

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/integrations` | Session | — | Get integration status |
| `PUT` | `/api/v1/integrations` | Session | `integrations:write` | Update integrations |
| `POST` | `/api/v1/integrations/test` | Session | `integrations:test` | Test Mattermost or Slack connection |
| `POST` | `/api/v1/integrations/slack/oauth/authorize` | Session | `integrations:write` | Initiate Slack OAuth flow |
| `GET` | `/api/v1/integrations/slack/oauth/callback` | Session | — | Slack OAuth callback |
| `POST` | `/api/v1/integrations/slack/disconnect` | Session | `integrations:write` | Disconnect Slack workspace |

## Users

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/users` | Session | `users:manage` | List users |
| `POST` | `/api/v1/users` | Session | `users:manage` | Create user |
| `PUT` | `/api/v1/users/{id}` | Session | `users:manage` | Update user |
| `DELETE` | `/api/v1/users/{id}` | Session | `users:manage` | Delete user |

### Linked Provider Accounts (Self)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/users/me/slack/authorize` | Session | Initiate user-to-Slack linking |
| `GET` | `/api/v1/users/me/slack/callback` | Session | OAuth callback for user Slack linking |
| `POST` | `/api/v1/users/me/slack/disconnect` | Session | Disconnect user's Slack account |
| `GET` | `/api/v1/users/me/google/authorize` | Session | Initiate user-to-Google linking |
| `GET` | `/api/v1/users/me/google/callback` | Session | OAuth callback for user Google linking |
| `POST` | `/api/v1/users/me/google/disconnect` | Session | Disconnect user's Google account |

## Knowledge

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/knowledge` | Session | — | List/search knowledge notes |
| `POST` | `/api/v1/knowledge` | Session | `knowledge:write` | Create knowledge note |
| `GET` | `/api/v1/knowledge/{id}` | Session | — | Get knowledge note |
| `PUT` | `/api/v1/knowledge/{id}` | Session | `knowledge:write` | Update knowledge note |
| `DELETE` | `/api/v1/knowledge/{id}` | Session | `knowledge:delete` | Delete knowledge note |

## Memories

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/memories` | Session | — | List/search memories |
| `POST` | `/api/v1/memories` | Session | `memories:write` | Create memory |
| `GET` | `/api/v1/memories/{id}` | Session | — | Get memory |
| `PUT` | `/api/v1/memories/{id}` | Session | `memories:write` | Update memory |
| `DELETE` | `/api/v1/memories/{id}` | Session | `memories:delete` | Delete memory |

## Notifications

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/notifications` | Session | List notifications for current user |
| `GET` | `/api/v1/notifications/unread-count` | Session | Get unread count |
| `POST` | `/api/v1/notifications/read-all` | Session | Mark all read |
| `POST` | `/api/v1/notifications/{id}/read` | Session | Mark one notification read |

## Notification Preferences

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/users/me/notification-preferences` | Session | Get preferences |
| `PUT` | `/api/v1/users/me/notification-preferences` | Session | Update preferences |
| `POST` | `/api/v1/users/me/notification-preferences/test` | Session | Send test notification |

## Incidents

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents` | Session | `incidents:read` | List incidents (status, severity, service_id, commander_id, search, dates) |
| `POST` | `/api/v1/incidents` | Session | `incidents:write` | Create manual incident |
| `GET` | `/api/v1/incidents/{id}` | Session | `incidents:read` | Get incident with timeline, roles, linked items |
| `PATCH` | `/api/v1/incidents/{id}` | Session | `incidents:write` | Update (title, description, severity, custom_fields) |
| `DELETE` | `/api/v1/incidents/{id}` | Session | `incidents:delete` | Delete incident |
| `POST` | `/api/v1/incidents/{id}/acknowledge` | Session | `incidents:command` | Acknowledge (stops escalation) |
| `POST` | `/api/v1/incidents/{id}/mitigate` | Session | `incidents:command` | Mark mitigated |
| `POST` | `/api/v1/incidents/{id}/resolve` | Session | `incidents:command` | Mark resolved |
| `POST` | `/api/v1/incidents/{id}/close` | Session | `incidents:command` | Mark closed |
| `POST` | `/api/v1/incidents/{id}/reopen` | Session | `incidents:command` | Reopen resolved/mitigated/closed |
| `POST` | `/api/v1/incidents/{id}/cancel` | Session | `incidents:command` | Cancel (false alarm) |
| `POST` | `/api/v1/incidents/{id}/escalate` | Session | `incidents:command` | Manual escalation trigger |

### Incident Timeline

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/timeline` | Session | `incidents:read` | Get structured timeline |
| `POST` | `/api/v1/incidents/{id}/timeline` | Session | `incidents:write` | Add manual timeline entry |

### Incident Thread

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/thread` | Session | `incidents:read` | Get incident investigation thread |
| `POST` | `/api/v1/incidents/{id}/thread/messages` | Session | `incidents:write` | Add thread message |

### Coordination

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/coordination/messages` | Session | `incidents:read` | List coordination messages |
| `POST` | `/api/v1/incidents/{id}/coordination/messages` | Session | `incidents:write` | Add coordination message |
| `GET` | `/api/v1/incidents/{id}/coordination/tasks` | Session | `incidents:read` | List coordination tasks |
| `POST` | `/api/v1/incidents/{id}/coordination/tasks` | Session | `incidents:write` | Create a coordination task |
| `PATCH` | `/api/v1/incidents/{id}/coordination/tasks/{taskId}` | Session | `incidents:write` | Update coordination task |

### Status Updates

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/status-updates` | Session | `incidents:read` | List status updates |
| `POST` | `/api/v1/incidents/{id}/status-updates` | Session | `incidents:command` | Create status update |

### Incident Document

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/document` | Session | `incidents:read` | Get document sections |
| `GET` | `/api/v1/incidents/{id}/document/{section}` | Session | `incidents:read` | Get one section |
| `PATCH` | `/api/v1/incidents/{id}/document/{section}` | Session | `incidents:write` | Update document section |

### ICS Roles

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/ics/roles` | Session | `incidents:read` | List ICS role assignments |
| `POST` | `/api/v1/incidents/{id}/ics/roles` | Session | `incidents:command` | Assign ICS role |
| `POST` | `/api/v1/incidents/{id}/ics/roles/{roleId}/end` | Session | `incidents:command` | End an active ICS role |
| `GET`/`PATCH` | `/api/v1/incidents/{id}/ics/document` | Session | `incidents:read`/`incidents:write` | Read/update ICS document sections |

### Triage Actions

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/begin-triage` | Session | `incidents:command` | Begin triage |
| `POST` | `/api/v1/incidents/{id}/promote` | Session | `incidents:command` | Promote to active |

### Linked Items

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/alerts` | Session | `incidents:read` | List linked alerts |
| `POST` | `/api/v1/incidents/{id}/alerts` | Session | `incidents:write` | Link alert |
| `DELETE` | `/api/v1/incidents/{id}/alerts/{alertNumber}` | Session | `incidents:write` | Unlink alert |
| `GET` | `/api/v1/incidents/{id}/investigations` | Session | `incidents:read` | List investigations under incident |

### Incident Channels & War Rooms

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/slack-channel` | Session | `incidents:command` | Create/link a Slack incident channel |
| `DELETE` | `/api/v1/incidents/{id}/slack-channel` | Session | `incidents:command` | Unlink Slack channel |
| `POST` | `/api/v1/incidents/{id}/google-meet` | Session | `incidents:command` | Create/link a Google Meet war room |
| `DELETE` | `/api/v1/incidents/{id}/google-meet` | Session | `incidents:command` | Unlink Google Meet war room |

### Incident Metrics

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/metrics` | Session | `incidents:read` | Aggregate metrics (MTTA, MTTR, MTTM, SLA) |

## Investigations

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/investigations/dead-lettered` | Session | `admin:access` | List dead-lettered investigations |

Investigation interactions for operators happen through the alert and incident threads above; for agents through the [Agent REST API](#agent-rest-api) and [Agent SSE](#agent-sse).

## Services

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/services` | Session | `services:read` | List services |
| `POST` | `/api/v1/services` | Session | `services:write` | Create service |
| `GET` | `/api/v1/services/{id}` | Session | `services:read` | Get service (includes status, dependencies) |
| `PATCH` | `/api/v1/services/{id}` | Session | `services:write` | Update service |
| `DELETE` | `/api/v1/services/{id}` | Session | `services:write` | Delete service |

### Dependencies

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/services/{id}/dependencies` | Session | `services:read` | Get dependency graph |
| `POST` | `/api/v1/services/{id}/dependencies` | Session | `services:write` | Add dependency |
| `DELETE` | `/api/v1/services/{id}/dependencies/{targetId}` | Session | `services:write` | Remove dependency |

### Service Incidents

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/services/{id}/incidents` | Session | `services:read` | List incidents affecting this service |

## Teams

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/teams` | Session | `oncall:read` | List teams |
| `POST` | `/api/v1/teams` | Session | `oncall:write` | Create team (auto-provisions an on-call schedule) |
| `GET` | `/api/v1/teams/{id}` | Session | `oncall:read` | Get team (includes members, escalation policy, schedules) |
| `PATCH` | `/api/v1/teams/{id}` | Session | `oncall:write` | Update team |
| `DELETE` | `/api/v1/teams/{id}` | Session | `oncall:write` | Delete team |

### Team Members

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/teams/{id}/members` | Session | `oncall:read` | List members with roles |
| `POST` | `/api/v1/teams/{id}/members` | Session | `oncall:write` | Add member (user_id, role) |
| `PATCH` | `/api/v1/teams/{id}/members/{userId}` | Session | `oncall:write` | Update member role |
| `DELETE` | `/api/v1/teams/{id}/members/{userId}` | Session | `oncall:write` | Remove member |

## Escalation Policies

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/escalation-policies` | Session | `escalation:read` | List escalation policies |
| `POST` | `/api/v1/escalation-policies` | Session | `escalation:write` | Create policy |
| `GET` | `/api/v1/escalation-policies/{id}` | Session | `escalation:read` | Get policy with levels and targets |
| `PATCH` | `/api/v1/escalation-policies/{id}` | Session | `escalation:write` | Update policy |
| `DELETE` | `/api/v1/escalation-policies/{id}` | Session | `escalation:write` | Delete policy |

## On-Call

Schedules are auto-provisioned one-per-team (creating a team also creates its schedule). Use the schedule endpoints to manage layers and overrides.

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/schedules` | Session | `oncall:read` | List schedules |
| `GET` | `/api/v1/on-call/schedules/{id}` | Session | `oncall:read` | Get schedule with layers |
| `PATCH` | `/api/v1/on-call/schedules/{id}` | Session | `oncall:write` | Update schedule layers |
| `GET` | `/api/v1/on-call/schedules/{id}/current` | Session | `oncall:read` | Current on-call for schedule |
| `GET` | `/api/v1/on-call/schedules/{id}/timeline` | Session | `oncall:read` | Next N rotations |

### Overrides

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/schedules/{id}/overrides` | Session | `oncall:read` | List overrides |
| `POST` | `/api/v1/on-call/schedules/{id}/overrides` | Session | `oncall:write` | Create override |
| `DELETE` | `/api/v1/on-call/overrides/{id}` | Session | `oncall:write` | Delete override |

### Handoffs

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/handoffs` | Session | `oncall:read` | List handoffs |
| `GET` | `/api/v1/on-call/handoffs/{id}` | Session | `oncall:read` | Get handoff |
| `GET` | `/api/v1/on-call/handoffs/pending` | Session | `oncall:read` | Pending handoffs for current user |
| `PATCH` | `/api/v1/on-call/handoffs/{id}/notes` | Session | `oncall:write` | Save handoff notes |
| `POST` | `/api/v1/on-call/handoffs/{id}/acknowledge` | Session | `oncall:write` | Acknowledge handoff |

### On-Call Lookup & Metrics

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/on-call/who-is-on-call` | Session | `oncall:read` | Global on-call status |
| `GET` | `/api/v1/on-call/me` | Session | — | My current/pending shifts |
| `GET` | `/api/v1/on-call/my-on-call` | Session | — | My on-call shifts (alternate) |
| `GET` | `/api/v1/on-call/metrics` | Session | `oncall:read` | Pager load metrics per shift |

## Post-Mortems

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/post-mortems` | Session | `postmortems:read` | List all post-mortems |
| `GET` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:read` | Get post-mortem |
| `POST` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:write` | Create post-mortem |
| `PATCH` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:write` | Update post-mortem |
| `DELETE` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:delete` | Delete post-mortem |
| `POST` | `/api/v1/incidents/{id}/post-mortem/submit-review` | Session | `postmortems:write` | Submit for review |
| `POST` | `/api/v1/incidents/{id}/post-mortem/approve` | Session | `postmortems:write` | Approve post-mortem |
| `POST` | `/api/v1/incidents/{id}/post-mortem/publish` | Session | `postmortems:write` | Publish post-mortem |

### Action Items

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/post-mortem/action-items` | Session | `postmortems:read` | List action items |
| `POST` | `/api/v1/incidents/{id}/post-mortem/action-items` | Session | `postmortems:write` | Create action item |
| `PATCH` | `/api/v1/post-mortem/action-items/{id}` | Session | `postmortems:write` | Update action item |
| `DELETE` | `/api/v1/post-mortem/action-items/{id}` | Session | `postmortems:write` | Delete action item |
| `GET` | `/api/v1/action-items` | Session | `postmortems:read` | All open action items |

## Playbooks

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/playbooks` | Session | `playbooks:read` | List playbooks (kind, service_id, tag, search) |
| `POST` | `/api/v1/playbooks` | Session | `playbooks:write` | Create playbook |
| `GET` | `/api/v1/playbooks/{id}` | Session | `playbooks:read` | Get playbook with steps |
| `PATCH` | `/api/v1/playbooks/{id}` | Session | `playbooks:write` | Update playbook |
| `DELETE` | `/api/v1/playbooks/{id}` | Session | `playbooks:write` | Delete playbook |
| `POST` | `/api/v1/playbooks/{id}/steps` | Session | `playbooks:write` | Add step |
| `PATCH` | `/api/v1/playbooks/{id}/steps/{stepId}` | Session | `playbooks:write` | Update step |
| `DELETE` | `/api/v1/playbooks/{id}/steps/{stepId}` | Session | `playbooks:write` | Delete step |
| `PUT` | `/api/v1/playbooks/{id}/steps/reorder` | Session | `playbooks:write` | Reorder steps |

## Triage

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/triage/rules` | Session | `triage:read` | List triage rules |
| `POST` | `/api/v1/triage/rules` | Session | `triage:write` | Create triage rule |
| `GET` | `/api/v1/triage/rules/{id}` | Session | `triage:read` | Get triage rule |
| `PUT` | `/api/v1/triage/rules/{id}` | Session | `triage:write` | Update triage rule |
| `DELETE` | `/api/v1/triage/rules/{id}` | Session | `triage:write` | Delete triage rule |
| `PUT` | `/api/v1/triage/rules/reorder` | Session | `triage:write` | Reorder triage rules |
| `GET` | `/api/v1/triage/results` | Session | `triage:read` | List triage results |
| `GET` | `/api/v1/triage/results/{id}` | Session | `triage:read` | Get triage result |
| `POST` | `/api/v1/triage/results/{id}` | Session | `triage:override` | Override triage decision |
| `GET` | `/api/v1/triage/stats` | Session | `triage:read` | Get triage accuracy stats |

## Heartbeats

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/heartbeats` | Session | `heartbeats:read` | List heartbeats |
| `POST` | `/api/v1/heartbeats` | Session | `heartbeats:write` | Create heartbeat |
| `GET` | `/api/v1/heartbeats/{id}` | Session | `heartbeats:read` | Get heartbeat |
| `PUT` | `/api/v1/heartbeats/{id}` | Session | `heartbeats:write` | Update heartbeat |
| `DELETE` | `/api/v1/heartbeats/{id}` | Session | `heartbeats:write` | Delete heartbeat |
| `POST` | `/api/v1/heartbeats/{id}/regenerate-token` | Session | `heartbeats:write` | Regenerate ping token |
| `GET` | `/api/v1/heartbeats/ping/{token}` | None | Dead-man's-switch ping (token-auth) |

## Status Pages

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/status-pages` | Session | `statuspages:read` | List status pages |
| `POST` | `/api/v1/status-pages` | Session | `statuspages:write` | Create status page |
| `GET` | `/api/v1/status-pages/slug/{slug}` | Session | `statuspages:read` | Public view by slug |
| `GET`/`PATCH`/`DELETE` | `/api/v1/status-pages/{id}` | Session | `statuspages:*` | Manage a status page and its components |

## OIDC SSO Providers

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/oidc/providers` | Session | `oidc:manage` | List configured OIDC providers |
| `POST` | `/api/v1/oidc/providers` | Session | `oidc:manage` | Create provider |
| `GET`/`PUT`/`DELETE` | `/api/v1/oidc/providers/{id}` | Session | `oidc:manage` | Manage a provider |

## Credential Providers

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/credential-providers` | Session | `credentials:read` | List credential providers |
| `POST` | `/api/v1/credential-providers` | Session | `credentials:manage` | Create provider |
| `GET`/`PATCH`/`DELETE` | `/api/v1/credential-providers/{id}` | Session | `credentials:read`/`manage` | Manage a provider |

## Shared Secrets

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/shared-secrets` | Session | `credentials:read` | List shared secrets |
| `POST` | `/api/v1/shared-secrets` | Session | `credentials:manage` | Create shared secret |
| `GET`/`PATCH`/`DELETE` | `/api/v1/shared-secrets/{id}` | Session | `credentials:read`/`manage` | Manage a shared secret |

## Personal Access Tokens

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/user/tokens` | Session | List your PATs |
| `POST` | `/api/v1/user/tokens` | Session | Create PAT |
| `DELETE` | `/api/v1/user/tokens/{id}` | Session | Revoke your PAT |
| `GET` | `/api/v1/admin/tokens` | Session | List all PATs (`tokens:manage`) |
| `DELETE` | `/api/v1/admin/tokens/{id}` | Session | Revoke any PAT (`tokens:manage`) |

## System Configuration

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/system/config` | Session | `system:read` | Get runtime system config |
| `PUT` | `/api/v1/system/config` | Session | `system:write` | Update system config |
| `GET` | `/api/v1/onboarding/status` | Session | `system:read` | Check if onboarding wizard is completed |
| `POST` | `/api/v1/onboarding/complete` | Session | `system:write` | Mark onboarding as completed |

## Dashboard

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/dashboard/stats` | Session | Aggregate dashboard counters |
| `GET` | `/api/v1/dashboard/daily-summary` | Session | Daily summary report |
| `POST` | `/api/v1/dashboard/daily-summary` | Session | Generate daily summary on demand |

## Channels & Destinations

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/channels` | Session | List Mattermost channels |
| `GET` | `/api/v1/destinations` | Session | List channels (supports `?provider=slack`) |

## Real-Time Events

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/events` | Session | SSE stream for frontend updates |

## Webhook Ingestion

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/webhooks/alerts` | Bearer token | Ingest alerting webhooks |
| `POST` | `/webhooks/mattermost` | Shared secret | Mattermost plugin webhook |
| `POST` | `/webhooks/slack` | Signing secret | Slack Events API webhook |

## Voice Callbacks

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/twilio/callback` | Twilio Signature | Twilio voice call IVR callback |
| `POST` | `/api/v1/telnyx/callback` | Telnyx Signature (Ed25519) | Telnyx voice call IVR callback |

## Health & Metrics

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | None | Liveness check |
| `GET` | `/api/v1/readiness` | None | Pipeline readiness + scheduler/correlator snapshot |
| `GET` | `/metrics` | None (gate at ingress) | Prometheus-format metrics |
| `GET` | `/debug/vars` | None (gate at ingress) | expvar metrics |

## Internal

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/internal/mm-plugin` | None | Serves Mattermost plugin tarball |
